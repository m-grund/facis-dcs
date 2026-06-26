package dcstodcssynchronizer

import (
	"net/http"
	"os"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	dcstodcsc "digital-contracting-service/gen/http/dcs_to_dcs/client"
	"digital-contracting-service/internal/base/conf"

	goahttp "goa.design/goa/v3/http"
)

type prefixDoer struct {
	prefix string
	inner  goahttp.Doer
}

func (d *prefixDoer) Do(req *http.Request) (*http.Response, error) {
	req.URL.Path = d.prefix + req.URL.Path
	return d.inner.Do(req)
}

func newDCSToDCSHttpClient(host string) *dcstodcs.Client {
	apiPath := os.Getenv("DCS_API_PATH")
	if apiPath == "" {
		apiPath = "/"
	}
	httpClient := &http.Client{Timeout: conf.HTTPClientTimeout()}
	doer := &prefixDoer{prefix: apiPath, inner: httpClient}

	c := dcstodcsc.NewClient(
		"http",
		host,
		doer,
		goahttp.RequestEncoder,
		goahttp.ResponseDecoder,
		false,
	)
	createEndpoint := c.Sync()
	return dcstodcs.NewClient(createEndpoint)
}
