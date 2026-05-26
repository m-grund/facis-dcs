Feature: Prometheus Monitoring
    Monitoring with Prometheus is enabled to collect metrics from the DCS components. This allows for performance tracking, alerting, and visualization of system health.

    Scenario: /metrics endpoint is exposed
        Given the DCS is deployed with Prometheus monitoring enabled
        When I access the /metrics endpoint of the DCS API
        Then I receive a response with Prometheus-formatted metrics data