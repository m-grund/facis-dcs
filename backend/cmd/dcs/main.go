package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	genauth "digital-contracting-service/gen/auth"
	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	externaltargetsystemapi "digital-contracting-service/gen/external_target_system_api"
	orchestrationwebhooks "digital-contracting-service/gen/orchestration_webhooks"
	pdfgeneration "digital-contracting-service/gen/pdf_generation"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	pg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/tsa"
	contractworkflowengine2 "digital-contracting-service/internal/contractworkflowengine"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/cryptoprovider"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/c2pa"
	pdfevent "digital-contracting-service/internal/pdfgeneration/event"
	"digital-contracting-service/internal/service"
	smrepo "digital-contracting-service/internal/signingmanagement/db/pg"
	"digital-contracting-service/internal/signingmanagement/dss"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/webhookplatform"
	"digital-contracting-service/migrations"
	"digital-contracting-service/migrations/fcschemas"

	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
	"goa.design/clue/debug"
	"goa.design/clue/log"
)

func main() {
	if err := loadDotenvIfPresent(); err != nil {
		fmt.Fprintf(os.Stderr, "startup configuration error: %v\n", err)
		os.Exit(1)
	}

	// Define command line flags, add any other flag required to configure the
	// service.
	var (
		hostF     = flag.String("host", "local", "Server host (valid values: local)")
		domainF   = flag.String("domain", "", "Host domain name (overrides host domain specified in service design)")
		httpPortF = flag.String("http-port", "", "HTTP port (overrides host HTTP port specified in service design)")
		secureF   = flag.Bool("secure", false, "Use secure scheme (https or grpcs)")
		dbgF      = flag.Bool("debug", false, "Log request and response bodies")
	)
	flag.Parse()

	// Setup logger. Replace logger with your own log package of choice.
	format := log.FormatJSON
	if log.IsTerminal() {
		format = log.FormatTerminal
	}
	ctx := log.Context(context.Background(), log.WithFormat(format))
	if *dbgF {
		ctx = log.Context(ctx, log.WithDebug())
		log.Debugf(ctx, "debug logs enabled")
	}
	log.Print(ctx, log.KV{K: "http-port", V: *httpPortF})

	db, err := NewDatabaseConnection()
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to database")
	}
	defer func(db *sqlx.DB) {
		err := db.Close()
		if err != nil {
			fmt.Printf("could not close database connection: %v\n", err)
		}
	}(db)

	log.Printf(ctx, "Connecting to database")

	// Run database migrations
	if err := migrations.Run(db); err != nil {
		log.Fatalf(ctx, err, "Could not run database migrations")
		os.Exit(1)
	}

	if os.Getenv("DCS_ISSUER") == "" {
		log.Printf(ctx, "DCS_ISSUER configuration missing: DCS_ISSUER will be set to localhost as issuer")
	} else {
		if strings.Contains(os.Getenv("DCS_ISSUER"), ":") {
			log.Fatalf(ctx, nil, "DCS_ISSUER must not contain service port")
		}
	}

	// Connect to NATS (use NATS_URL env var or default)
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	cepPubClient, err := event.NewNatsPubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to events publisher")
	}
	defer func(cepPubClient *event.CloudEventPubClient) {
		err := cepPubClient.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close cloud event publisher")
		}
	}(cepPubClient)

	// Initialize OIDC validator and JWT authenticator.
	hydraIssuerURL := os.Getenv("HYDRA_ISSUER_URL")
	hydraClientID := os.Getenv("HYDRA_CLIENT_ID")
	if hydraIssuerURL == "" || hydraClientID == "" {
		log.Fatalf(ctx, nil, "Hydra configuration missing: HYDRA_ISSUER_URL and HYDRA_CLIENT_ID must be set")
	}
	hydraJWTValidator, err := middleware.NewHydraJWTValidator(ctx, middleware.HydraJWTConfig{
		IssuerURL: hydraIssuerURL,
		ClientID:  hydraClientID,
	})
	if err != nil {
		log.Fatalf(ctx, err, "failed to initialize Hydra JWT validator")
	}

	// Initialize IPFS client
	ipfsTenantBaseURL := os.Getenv("IPFS_TENANT_BASE_URL")
	mfsBaseURL := os.Getenv("IPFS_MFS_BASE_URL")
	if ipfsTenantBaseURL == "" || mfsBaseURL == "" {
		log.Fatalf(ctx, nil, "IPFS configuration missing: IPFS_TENANT_BASE_URL and IPFS_MFS_BASE_URL environment variables must be specified")
	}
	ipfsAPIClient := ipfs.NewClient(ipfsTenantBaseURL, mfsBaseURL)

	aAttemptRepo := &pg.PostgresAccessAttemptRepo{}
	lockRepo := &pg.PostgresIPLockoutRepo{}
	jwtAuth := auth.NewJWTAuthenticator(hydraJWTValidator, db, aAttemptRepo, lockRepo)

	ctRepo := tplrepo.PostgresContractTemplateRepo{}
	ctRTRepo := tplrepo.PostgresReviewTaskRepo{}
	ctATRepo := tplrepo.PostgresApprovalTaskRepo{}

	cweRepo := cwerepo.PostgresContractRepo{}
	cweRTRepo := cwerepo.PostgresReviewTaskRepo{}
	cweATRepo := cwerepo.PostgresApprovalTaskRepo{}
	cweNTRepo := cwerepo.PostgresNegotiationTaskRepo{}
	cweNRepo := cwerepo.PostgresNegotiationRepo{}
	cweCTRepo := cwerepo.PostgresContractTemplateRepo{}
	cweCronJob := contractworkflowengine2.CronJob{DB: db, CRepo: &cweRepo}
	cweCronJob.Start(ctx, db)

	smCRepo := smrepo.PostgresContractRepo{
		IPFSClient: ipfsAPIClient,
	}

	aRepo := pq.PostgresAuditTrailRepository{}

	tsaURL := os.Getenv("TSA_URL")
	if tsaURL == "" {
		log.Fatalf(ctx, nil, "TSA_URL is not set")
	}
	tsaClient, err := tsa.NewClient(tsaURL)
	if err != nil {
		log.Fatalf(ctx, err, "failed to initialize TSA client")
	}

	outboxProcessor := event.OutboxProcessor{
		DB:         db,
		PubClient:  cepPubClient,
		IPFSClient: ipfsAPIClient,
		ARepo:      &aRepo,
		TSAClient:  tsaClient,
	}
	err = outboxProcessor.Start(ctx)
	if err != nil {
		log.Fatalf(ctx, err, "failed to start outbox processor")
	}

	auditTrailReader := base.AuditTrailReader{
		IPFSClient: ipfsAPIClient,
		ARepo:      &aRepo,
	}

	archiveNotaryURL := strings.TrimSpace(os.Getenv("ORCE_ARCHIVE_NOTARY_URL"))
	var archiveNotaryClient cwecommand.ArchiveNotary
	if archiveNotaryURL != "" {
		archiveNotaryClient = cwecommand.NewHTTPArchiveNotaryClient(archiveNotaryURL)
	}
	// Initialize the Federated Catalogue client.
	fcURL := os.Getenv("FEDERATED_CATALOGUE_API_URL")
	fcClientID := os.Getenv("FEDERATED_CATALOGUE_CLIENT_ID")
	fcClientSecret := os.Getenv("FEDERATED_CATALOGUE_CLIENT_SECRET")
	var templateCatalogueClient *fcclient.FederatedCatalogueClient
	if fcURL != "" && fcClientID != "" && fcClientSecret != "" {
		fcRealmURL := strings.TrimSpace(os.Getenv("FC_KEYCLOAK_REALM_URL"))
		if fcRealmURL == "" {
			log.Fatalf(ctx, nil, "Federated Catalogue requires FC_KEYCLOAK_REALM_URL (Keycloak for FC only, not Hydra)")
		}
		templateCatalogueClient, err = fcclient.NewFederatedCatalogueClient(fcclient.Config{
			APIURL:           fcURL,
			KeycloakRealmURL: fcRealmURL,
			ClientID:         fcClientID,
			ClientSecret:     fcClientSecret,
		})
		if err != nil {
			log.Fatalf(ctx, err, "failed to initialize Federated Catalogue client")
		}
		if err := fcschemas.SyncWithRetry(ctx, templateCatalogueClient); err != nil {
			log.Fatalf(ctx, err, "failed to sync federated catalogue schemas")
		}
	}

	// Initialize the webhook platform (ORCE integration).
	webhookStore := webhookplatform.NewSubscriptionStore()
	webhookDispatcher := webhookplatform.NewDispatcher(webhookStore)
	webhookPlatform := webhookplatform.New(
		webhookStore,
		webhookDispatcher,
		func(ctx context.Context, token string) (string, error) {
			info, err := hydraJWTValidator.ValidateToken(ctx, token)
			if err != nil {
				return "", err
			}
			return info.ParticipantDID, nil
		},
		nil,
	)

	// Start the NATS→Webhook bridge: automatically fans out to all registered
	// webhook subscribers whenever a DCS lifecycle event fires on the event bus.
	webhookSubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not create webhook NATS subscriber")
	}
	defer func(webhookSubClient *event.CloudEventSubClient) {
		err := webhookSubClient.Close()
		if err != nil {
			log.Errorf(ctx, err, "failed to close webhook subscriber")
		}
	}(webhookSubClient)
	go func() {
		if err := webhookplatform.StartNATSBridge(webhookSubClient, webhookDispatcher); err != nil {
			log.Fatalf(ctx, err, "Could not start webhook NATS bridge")
		}
	}()

	// Initialize Crypto Provider Service client for C2PA signing.
	cryptoProviderURL := os.Getenv("CRYPTO_PROVIDER_URL")
	cryptoProviderNamespace := os.Getenv("CRYPTO_PROVIDER_NAMESPACE")
	cryptoProviderKey := os.Getenv("CRYPTO_PROVIDER_KEY")
	cryptoProviderCertChainFile := strings.TrimSpace(os.Getenv("CRYPTO_PROVIDER_CERT_CHAIN_FILE"))
	issuerDID := os.Getenv("ISSUER_DID")
	cryptoClient := cryptoprovider.NewClient(cryptoProviderURL, cryptoProviderNamespace, cryptoProviderKey)

	if cryptoProviderCertChainFile == "" {
		log.Fatalf(ctx, nil, "CRYPTO_PROVIDER_CERT_CHAIN_FILE is required")
	}
	if err := cryptoClient.SetCertificateChainFromPEMFile(cryptoProviderCertChainFile); err != nil {
		log.Fatalf(ctx, err, "load crypto provider certificate chain from file")
	}

	tsaCfg := c2pa.TSAConfig{URL: os.Getenv("TSA_URL")}

	// Probe crypto provider liveness before accepting traffic.
	if cryptoProviderURL == "" {
		log.Fatalf(ctx, nil, "CRYPTO_PROVIDER_URL is required")
	}
	if err := probeHTTP(cryptoProviderURL + "/readiness"); err != nil {
		log.Fatalf(ctx, err, "crypto provider not reachable at %s", cryptoProviderURL)
	}

	// Initialize OCM-W Status List Service client (DCS-OR-C2PA-005).
	statusListServiceURL := os.Getenv("STATUSLIST_SERVICE_URL")
	if statusListServiceURL == "" {
		log.Fatalf(ctx, nil, "STATUSLIST_SERVICE_URL is required (DCS-OR-C2PA-005)")
	}
	if err := probeHTTPAny(statusListServiceURL+"/health", statusListServiceURL+"/v1/metrics/health"); err != nil {
		log.Fatalf(ctx, err, "status list service not reachable at %s", statusListServiceURL)
	}
	statusListTenantID := os.Getenv("STATUSLIST_TENANT_ID") // defaults to "default" when empty
	statusListPublisher := c2pa.NewOCMWStatusListPublisher(statusListServiceURL, issuerDID, statusListTenantID)

	// Initialize the service.
	var (
		authSvc                         genauth.Service
		contractStorageArchiveSvc       contractstoragearchive.Service
		contractWorkflowEngineSvc       contractworkflowengine.Service
		dcsToDcsSvc                     dcstodcs.Service
		externalTargetSystemAPISvc      externaltargetsystemapi.Service
		orchestrationWebhooksSvc        orchestrationwebhooks.Service
		pdfGenerationSvc                pdfgeneration.Service
		processAuditAndComplianceSvc    processauditandcompliance.Service
		signatureManagementSvc          signaturemanagement.Service
		templateCatalogueIntegrationSvc templatecatalogueintegration.Service
		templateRepositorySvc           templaterepository.Service
	)
	{
		presentationRepo := pg.NewPostgresPresentationAttemptRepo(db)
		authSvc = service.NewAuth(presentationRepo)
		contractStorageArchiveSvc = service.NewContractStorageArchive(db, jwtAuth, &cweRepo)
		contractWorkflowEngineSvc = service.NewContractWorkflowEngine(db, jwtAuth, &cweRepo, &cweRTRepo, &cweATRepo, &cweNTRepo, &cweNRepo, &cweCTRepo, templateCatalogueClient, auditTrailReader, ipfsAPIClient, archiveNotaryClient, tsaClient)
		dcsToDcsSvc = service.NewDcsToDcs(jwtAuth)
		externalTargetSystemAPISvc = service.NewExternalTargetSystemAPI(jwtAuth)
		orchestrationWebhooksSvc = service.NewOrchestrationWebhooks(jwtAuth)
		pdfGenerationSvc = service.NewPDFGeneration(db, jwtAuth, ipfsAPIClient, &cweRepo, &ctRepo, cryptoClient, tsaCfg, issuerDID, c2pa.NewLocalVCIssuer(cryptoClient, issuerDID, statusListPublisher))
		processAuditAndComplianceSvc = service.NewProcessAuditAndCompliance(db, jwtAuth, auditTrailReader, &ctRepo, &cweRepo)
		signatureManagementSvc = service.NewSignatureManagement(db, jwtAuth, &smCRepo, auditTrailReader, dss.StubClient{}, ipfsAPIClient)
		templateCatalogueIntegrationSvc = service.NewTemplateCatalogueIntegration(jwtAuth, templateCatalogueClient)
		templateRepositorySvc = service.NewTemplateRepository(db, jwtAuth, &ctRepo, &ctRTRepo, &ctATRepo, templateCatalogueClient, auditTrailReader)
	}

	// Channel used by background workers and signal handler to notify main to exit.
	errc := make(chan error)

	// Start the PDF lifecycle C2PA subscriber (appends C2PA assertions on state changes).
	// Only start when a real signing URL is configured; without one, the subscriber
	// would attempt signing on every CWE event and log spurious HTTP errors.
	pdfSubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not create PDF generation NATS subscriber")
	}
	defer func(pdfSubClient *event.CloudEventSubClient) {
		err := pdfSubClient.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close PDF subscriber")
		}
	}(pdfSubClient)
	pdfSub := &pdfevent.Subscriber{
		DB:         db,
		IPFSClient: ipfsAPIClient,
		CRepo:      &cweRepo,
		TRepo:      &ctRepo,
		Signer:     cryptoClient,
		TSACfg:     tsaCfg,
		IssuerDID:  issuerDID,
		VCIssuer:   c2pa.NewLocalVCIssuer(cryptoClient, issuerDID, statusListPublisher),
	}
	go func() {
		if err := pdfSub.Start(pdfSubClient); err != nil {
			errc <- fmt.Errorf("could not start PDF generation subscriber: %w", err)
		}
	}()

	// Wrap the service in endpoints that can be invoked from other service
	// potentially running in different processes.
	var (
		authEndpoints                         *genauth.Endpoints
		contractStorageArchiveEndpoints       *contractstoragearchive.Endpoints
		contractWorkflowEngineEndpoints       *contractworkflowengine.Endpoints
		dcsToDcsEndpoints                     *dcstodcs.Endpoints
		externalTargetSystemAPIEndpoints      *externaltargetsystemapi.Endpoints
		orchestrationWebhooksEndpoints        *orchestrationwebhooks.Endpoints
		pdfGenerationEndpoints                *pdfgeneration.Endpoints
		processAuditAndComplianceEndpoints    *processauditandcompliance.Endpoints
		signatureManagementEndpoints          *signaturemanagement.Endpoints
		templateCatalogueIntegrationEndpoints *templatecatalogueintegration.Endpoints
		templateRepositoryEndpoints           *templaterepository.Endpoints
	)
	{
		authEndpoints = genauth.NewEndpoints(authSvc)
		authEndpoints.Use(debug.LogPayloads())
		authEndpoints.Use(log.Endpoint)
		contractStorageArchiveEndpoints = contractstoragearchive.NewEndpoints(contractStorageArchiveSvc)
		contractStorageArchiveEndpoints.Use(debug.LogPayloads())
		contractStorageArchiveEndpoints.Use(log.Endpoint)
		contractWorkflowEngineEndpoints = contractworkflowengine.NewEndpoints(contractWorkflowEngineSvc)
		contractWorkflowEngineEndpoints.Use(debug.LogPayloads())
		contractWorkflowEngineEndpoints.Use(log.Endpoint)
		dcsToDcsEndpoints = dcstodcs.NewEndpoints(dcsToDcsSvc)
		dcsToDcsEndpoints.Use(debug.LogPayloads())
		dcsToDcsEndpoints.Use(log.Endpoint)
		externalTargetSystemAPIEndpoints = externaltargetsystemapi.NewEndpoints(externalTargetSystemAPISvc)
		externalTargetSystemAPIEndpoints.Use(debug.LogPayloads())
		externalTargetSystemAPIEndpoints.Use(log.Endpoint)
		orchestrationWebhooksEndpoints = orchestrationwebhooks.NewEndpoints(orchestrationWebhooksSvc)
		orchestrationWebhooksEndpoints.Use(debug.LogPayloads())
		orchestrationWebhooksEndpoints.Use(log.Endpoint)
		pdfGenerationEndpoints = pdfgeneration.NewEndpoints(pdfGenerationSvc)
		pdfGenerationEndpoints.Use(debug.LogPayloads())
		pdfGenerationEndpoints.Use(log.Endpoint)
		processAuditAndComplianceEndpoints = processauditandcompliance.NewEndpoints(processAuditAndComplianceSvc)
		processAuditAndComplianceEndpoints.Use(debug.LogPayloads())
		processAuditAndComplianceEndpoints.Use(log.Endpoint)
		signatureManagementEndpoints = signaturemanagement.NewEndpoints(signatureManagementSvc)
		signatureManagementEndpoints.Use(debug.LogPayloads())
		signatureManagementEndpoints.Use(log.Endpoint)
		templateCatalogueIntegrationEndpoints = templatecatalogueintegration.NewEndpoints(templateCatalogueIntegrationSvc)
		templateCatalogueIntegrationEndpoints.Use(debug.LogPayloads())
		templateCatalogueIntegrationEndpoints.Use(log.Endpoint)
		templateRepositoryEndpoints = templaterepository.NewEndpoints(templateRepositorySvc)
		templateRepositoryEndpoints.Use(debug.LogPayloads())
		templateRepositoryEndpoints.Use(log.Endpoint)
	}

	// Setup interrupt handler. This optional step configures the process so
	// that SIGINT and SIGTERM signals cause the service to stop gracefully.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("%s", <-c)
	}()

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)

	// Start the servers and send errors (if any) to the error channel.
	switch *hostF {
	case "local":
		{
			addr := "http://0.0.0.0:8991"
			u, err := url.Parse(addr)
			if err != nil {
				log.Fatalf(ctx, err, "invalid URL %#v\n", addr)
			}
			if *secureF {
				u.Scheme = "https"
			}
			if *domainF != "" {
				u.Host = *domainF
			}
			if *httpPortF != "" {
				h, _, err := net.SplitHostPort(u.Host)
				if err != nil {
					log.Fatalf(ctx, err, "invalid URL %#v\n", u.Host)
				}
				u.Host = net.JoinHostPort(h, *httpPortF)
			} else if u.Port() == "" {
				u.Host = net.JoinHostPort(u.Host, "80")
			}
			handleHTTPServer(ctx, u, authEndpoints, contractStorageArchiveEndpoints, contractWorkflowEngineEndpoints, dcsToDcsEndpoints, externalTargetSystemAPIEndpoints, orchestrationWebhooksEndpoints, pdfGenerationEndpoints, processAuditAndComplianceEndpoints, signatureManagementEndpoints, templateCatalogueIntegrationEndpoints, templateRepositoryEndpoints, webhookPlatform, &wg, errc, *dbgF)
		}

	default:
		log.Fatal(ctx, fmt.Errorf("invalid host argument: %q (valid hosts: local)", *hostF))
	}

	// Wait for signal.
	log.Printf(ctx, "exiting (%v)", <-errc)

	// Send cancellation signal to the goroutines.
	cancel()

	wg.Wait()
	log.Printf(ctx, "exited")
}
