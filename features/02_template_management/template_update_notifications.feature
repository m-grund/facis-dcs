# DCS-FR-TR-22: the system SHOULD notify Template Users when a contract
# template they have used has been updated or deprecated. The DCS webhook
# platform (backend/internal/webhookplatform, mounted at /orce/ on the
# service root) fans every template lifecycle event out to registered
# subscribers; the ORCE monitoring flow (charts/orce/flows/
# event-webhook-orce-flow.json, POST /dcs-dispatch) is the deployed
# receiver these scenarios deliver to. Each notification payload carries
# the template DID, which is how a subscriber matches notifications to the
# templates it has used. GET /deliveries is the platform's monitoring
# surface: per-notification callback URL, HTTP status, and acknowledgement.

@UC-02 @DCS-FR-TR-22
Feature: Template-update notifications through the webhook platform

  Scenario: A subscriber is notified when a template it uses is updated
    Given I am authenticated with roles: "Template Creator"
    And template "Notify Update Template" is in "Draft" status
    And a webhook subscription for "template.updated" events pointing at the ORCE monitoring flow
    When I update template "Notify Update Template" name to "Notify Update Template v2"
    Then get http 200:Success code
    And the "template.updated" notification for template "Notify Update Template" is delivered to the ORCE receiver

  Scenario: Subscribing to an event outside the published catalogue is rejected
    When a webhook subscription for the unknown event "template.exploded" is attempted
    Then the webhook subscription is rejected as unknown

  Scenario: The webhook platform rejects unauthenticated subscriptions
    When an unauthenticated webhook subscription for "template.updated" is attempted
    Then the webhook subscription is rejected as unauthorized
