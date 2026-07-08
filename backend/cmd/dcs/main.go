package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	dcstodcs2 "digital-contracting-service/internal/dcstodcs"
	dcstodcsdb "digital-contracting-service/internal/dcstodcs/db"
	pq2 "digital-contracting-service/internal/dcstodcs/db/pg"

	"digital-contracting-service/internal/signingmanagement/dss"

	didservice "digital-contracting-service/gen/did_service"

	genauth "digital-contracting-service/gen/auth"
	c2paservice "digital-contracting-service/gen/c2_pa_service"
	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	pdfgeneration "digital-contracting-service/gen/pdf_generation"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	pg "digital-contracting-service/internal/auth/db/pq"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/kv"
	"digital-contracting-service/internal/base/tsa"
	contractworkflowengine2 "digital-contracting-service/internal/contractworkflowengine"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/cryptoprovider"
	"digital-contracting-service/internal/middleware"
	pdfevent "digital-contracting-service/internal/pdfgeneration/event"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/service"
	smrepo "digital-contracting-service/internal/signingmanagement/db/pg"
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return false
}

func main() {
	// Define command line flags, add any other flag required to configure the
	// service.
	var (
		hostF     = flag.String("host", "local", "Server host (valid values: local)")
		domainF   = flag.String("domain", "", "Host domain name (overrides host domain specified in service design)")
		httpPortF = flag.String("http-port", "", "HTTP port (overrides host HTTP port specified in service design)")
		secureF   = flag.Bool("secure", false, "Use secure scheme (https or grpcs)")
		dbgF      = flag.Bool("debug", false, "Log request and response bodies")
		envF      = flag.String("env", "", "Set environment file for the service")
	)
	flag.Parse()

	if envF != nil && *envF != "" {
		if err := loadDotenvFile(*envF); err != nil {
			_, err := fmt.Fprintf(os.Stderr, "startup configuration error: %v\n", err)
			if err != nil {
				return
			}
			os.Exit(1)
		}
	} else {
		if err := loadDotenvIfPresent(); err != nil {
			_, err := fmt.Fprintf(os.Stderr, "startup configuration error: %v\n", err)
			if err != nil {
				return
			}
			os.Exit(1)
		}
	}

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

	didFilePath := os.Getenv("DCS_DID")
	if didFilePath == "" || !fileExists(didFilePath) {
		log.Printf(ctx, "DCS_DID configuration or file is missing")
	}

	privateKeyPath := os.Getenv("DCS_PRIVATE_KEY")
	if privateKeyPath == "" || !fileExists(privateKeyPath) {
		log.Printf(ctx, "DCS_PRIVATE_KEY configuration or file is missing")
	}

	log.Printf(ctx, "Reading did.json")
	didDocument, err := identity.NewDIDDocument(didFilePath, privateKeyPath)
	if err != nil {
		log.Fatalf(ctx, err, "Could not read did document")
	}

	var euTrustPool *identity.EUTrustPool
	if base.GetEnvOrDefault("DCS_FORCE_EIDAS_CERT", false) {
		log.Printf(ctx, "Start building EU trust pool")
		trustPool := identity.NewEUTrustPool()
		if err := trustPool.Refresh(ctx); err != nil {
			log.Fatalf(ctx, err, "Building EU trust pool")
		}
		count, _, errs := trustPool.Stats()
		log.Printf(ctx, "EU trust pool ready: %d certificates (%d lists skipped)", count, len(errs))

		// Keep it fresh in the background.
		go trustPool.StartAutoRefresh(ctx, identity.DefaultRefreshInterval)

		euTrustPool = trustPool
	}

	err = didDocument.VerifyEIDASCertificate(euTrustPool)
	if err != nil {
		log.Fatalf(ctx, err, "Could not verify certificate")
	}

	// Initialize OIDC validator and JWT authenticator.
	authCfg, err := loadAuthConfig(ctx)
	if err != nil {
		log.Fatalf(ctx, err, "Could not load auth config")
	}
	hydraJWTValidator, err := middleware.NewHydraJWTValidator(ctx, middleware.HydraJWTConfig{
		PublicIssuerURL:   authCfg.Hydra.PublicIssuerURL(),
		InternalIssuerURL: authCfg.Hydra.InternalIssuerURL(),
		ClientID:          authCfg.Hydra.ClientID(),
	})
	if err != nil {
		log.Fatalf(ctx, err, "Failed to initialize Hydra JWT validator")
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

	aRepo := pq.PostgresAuditTrailRepository{}

	tsaURL := os.Getenv("TSA_URL")
	if tsaURL == "" {
		log.Fatalf(ctx, nil, "TSA_URL is not set")
	}
	tsaClient, err := tsa.NewClient(tsaURL)
	if err != nil {
		log.Fatalf(ctx, err, "failed to initialize TSA client")
	}

	// Connect to NATS (use NATS_URL env var or default)
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	cepPubClient, err := event.NewNatsPubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to events bus")
	}
	defer func(client *event.CloudEventPubClient) {
		err := client.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close cloud event bus client")
		}
	}(cepPubClient)

	did, err := didDocument.GetID()
	if err != nil {
		log.Fatalf(ctx, err, "could not read DID")
	}

	outboxProcessor := event.OutboxProcessor{
		DB:           db,
		CEPPubClient: cepPubClient,
		IPFSClient:   ipfsAPIClient,
		ARepo:        &aRepo,
		TSAClient:    tsaClient,
	}
	err = outboxProcessor.Start(ctx, did)
	if err != nil {
		log.Fatalf(ctx, err, "failed to start outbox processor")
	}

	cepSubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to events bus")
	}
	defer func(client *event.CloudEventSubClient) {
		err := client.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close cloud event bus client")
		}
	}(cepSubClient)

	syncRepo := pq2.PostgresSyncRepository{}
	if err := seedTrustedPeersFromEnv(ctx, db, &syncRepo); err != nil {
		log.Fatalf(ctx, err, "failed to seed trusted peers from DCS_TRUSTED_PEERS")
	}
	dcsToDcsSynchronizer := dcstodcs2.DCSToDCSSynchronizer{
		DB:          db,
		CRepo:       &cweRepo,
		NRepo:       cweNRepo,
		NTRepo:      &cweNTRepo,
		RTRepo:      &cweRTRepo,
		ATRepo:      &cweATRepo,
		SRepo:       &syncRepo,
		DIDDocument: *didDocument,
	}
	dcsToDcsSynchronizer.StartSynchronizerJob(ctx, cepSubClient)

	if os.Getenv("DCS_DEBUG_EVENTING") == "true" {
		event.StartEventLogger(ctx, cepSubClient)
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
	statusListPublisher := provenance.NewOCMWStatusListPublisher(statusListServiceURL, issuerDID, statusListTenantID)

	// Initialize pdf-core client (PDF rendering + C2PA provenance microservice).
	pdfCoreURL := os.Getenv("PDF_CORE_URL")
	if pdfCoreURL == "" {
		log.Fatalf(ctx, nil, "PDF_CORE_URL is required")
	}
	if err := probeHTTP(pdfCoreURL + "/version"); err != nil {
		log.Fatalf(ctx, err, "pdf-core not reachable at %s", pdfCoreURL)
	}
	pdfCoreClient := pdfcore.New(pdfCoreURL)

	smCRepo := smrepo.PostgresContractRepo{
		IPFSClient: ipfsAPIClient,
		PDFCore:    pdfCoreClient,
	}

	didService, err := service.NewDIDService(*didDocument)
	if err != nil {
		log.Fatalf(ctx, err, "failed to create did service")
	}

	// Initialize the service.
	var (
		authSvc                         genauth.Service
		contractStorageArchiveSvc       contractstoragearchive.Service
		contractWorkflowEngineSvc       contractworkflowengine.Service
		dcsToDcsSvc                     dcstodcs.Service
		pdfGenerationSvc                pdfgeneration.Service
		processAuditAndComplianceSvc    processauditandcompliance.Service
		signatureManagementSvc          signaturemanagement.Service
		templateCatalogueIntegrationSvc templatecatalogueintegration.Service
		templateRepositorySvc           templaterepository.Service
		didSrv                          didservice.Service
		c2paSvc                         c2paservice.Service
	)
	{
		presentationRepo := pg.NewPostgresPresentationAttemptRepo(db)
		authSvc, err = service.NewAuth(db, presentationRepo, authCfg)
		if err != nil {
			log.Fatalf(ctx, err, "auth service init failed")
		}

		// The JAR signing public key cache is a required dependency of the signer:
		// no cache, no app.
		if authCfg.RequestSigner != nil {
			vaultSigner, ok := authCfg.RequestSigner.(*oid4vprequest.VaultTransitSigner)
			if !ok {
				log.Fatalf(ctx, fmt.Errorf("oid4vp request signer %T does not support the public key cache", authCfg.RequestSigner), "auth service init failed")
			}
			vaultSigner.SetPublicKeyCache(kv.NewStore(db), 0)
		}

		contractStorageArchiveSvc = service.NewContractStorageArchive(db, jwtAuth, &cweRepo, *didDocument)
		contractWorkflowEngineSvc = service.NewContractWorkflowEngine(db, jwtAuth, &cweRepo, &cweRTRepo, &cweATRepo, &cweNTRepo, &cweNRepo, &cweCTRepo, &syncRepo, euTrustPool, templateCatalogueClient, auditTrailReader, *didDocument, ipfsAPIClient, archiveNotaryClient, tsaClient)
		dcsToDcsSvc = service.NewDcsToDcs(db, jwtAuth, &cweRepo, &cweRTRepo, &cweATRepo, &cweNTRepo, &cweNRepo, &cweCTRepo, &syncRepo, euTrustPool, *didDocument, ipfsAPIClient)
		pdfGenerationSvc = service.NewPDFGeneration(db, jwtAuth, ipfsAPIClient, &cweRepo, &ctRepo, &smCRepo, pdfCoreClient, issuerDID, provenance.NewLocalVCIssuer(cryptoClient, issuerDID, statusListPublisher))
		c2paSvc = service.NewC2PAService(db, ipfsAPIClient, &cweRepo, pdfCoreClient, issuerDID, provenance.NewLocalVCIssuer(cryptoClient, issuerDID, statusListPublisher))
		processAuditAndComplianceSvc = service.NewProcessAuditAndCompliance(db, jwtAuth, auditTrailReader, &ctRepo, &cweRepo)
		signatureManagementSvc = service.NewSignatureManagement(db, jwtAuth, &smCRepo, auditTrailReader, dss.StubClient{}, ipfsAPIClient, pdfCoreClient)
		templateCatalogueIntegrationSvc = service.NewTemplateCatalogueIntegration(jwtAuth, templateCatalogueClient)
		templateRepositorySvc = service.NewTemplateRepository(db, jwtAuth, &ctRepo, &ctRTRepo, &ctATRepo, templateCatalogueClient, auditTrailReader)
		didSrv = didService
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
		PDFCore:    pdfCoreClient,
		IssuerDID:  issuerDID,
		VCIssuer:   provenance.NewLocalVCIssuer(cryptoClient, issuerDID, statusListPublisher),
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
		pdfGenerationEndpoints                *pdfgeneration.Endpoints
		processAuditAndComplianceEndpoints    *processauditandcompliance.Endpoints
		signatureManagementEndpoints          *signaturemanagement.Endpoints
		templateCatalogueIntegrationEndpoints *templatecatalogueintegration.Endpoints
		templateRepositoryEndpoints           *templaterepository.Endpoints
		didEntpoints                          *didservice.Endpoints
		c2paEndpoints                         *c2paservice.Endpoints
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
		didEntpoints = didservice.NewEndpoints(didSrv)
		didEntpoints.Use(debug.LogPayloads())
		didEntpoints.Use(log.Endpoint)
		c2paEndpoints = c2paservice.NewEndpoints(c2paSvc)
		c2paEndpoints.Use(debug.LogPayloads())
		c2paEndpoints.Use(log.Endpoint)
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

	address := "http://0.0.0.0:8991"
	if os.Getenv("DCS_BACKEND_PORT") != "" {
		address = fmt.Sprintf("http://0.0.0.0:%s", os.Getenv("DCS_BACKEND_PORT"))
	}

	// Start the servers and send errors (if any) to the error channel.
	switch *hostF {
	case "local":
		{
			addr := address
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
			handleHTTPServer(ctx, u, authEndpoints, contractStorageArchiveEndpoints, contractWorkflowEngineEndpoints, dcsToDcsEndpoints, pdfGenerationEndpoints, processAuditAndComplianceEndpoints, signatureManagementEndpoints, templateCatalogueIntegrationEndpoints, templateRepositoryEndpoints, didEntpoints, c2paEndpoints, webhookPlatform, &wg, errc, *dbgF)
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

// seedTrustedPeersFromEnv upserts every comma-separated peer DID listed in
// DCS_TRUSTED_PEERS into the trusted_peers allowlist at startup (NFR-BR-08:
// exchanges only between verified parties). Idempotent: re-running with the
// same env var value is a no-op thanks to UpsertTrustedPeer's
// ON CONFLICT (peer_did) DO NOTHING. A no-op (nothing logged/inserted) when
// the env var is unset or empty.
func seedTrustedPeersFromEnv(ctx context.Context, database *sqlx.DB, sRepo dcstodcsdb.SyncRepository) error {
	raw := os.Getenv("DCS_TRUSTED_PEERS")
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var seeded []string
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Errorf(ctx, err, "could not rollback trusted-peer seeding transaction")
		}
	}()

	for _, part := range strings.Split(raw, ",") {
		peerDID := strings.TrimSpace(part)
		if peerDID == "" {
			continue
		}
		if err := sRepo.UpsertTrustedPeer(ctx, tx, peerDID); err != nil {
			return fmt.Errorf("could not seed trusted peer %q: %w", peerDID, err)
		}
		seeded = append(seeded, peerDID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit trusted-peer seeding transaction: %w", err)
	}

	if len(seeded) > 0 {
		log.Printf(ctx, "seeded %d trusted peer(s) from DCS_TRUSTED_PEERS: %v", len(seeded), seeded)
	}

	return nil
}
