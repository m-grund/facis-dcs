package design

import (
	. "goa.design/goa/v3/dsl"
)

// Auth Service — Hydra OIDC and OpenID4VP authentication endpoints.
var _ = Service("Auth", func() {
	Description("Authentication endpoints for Hydra OIDC session management and OpenID4VP login.")

	Method("login", func() {
		Description("Starts the Hydra OIDC login flow with an OpenID4VP presentation request.")
		NoSecurity()
		Result(func() {
			Attribute("request_uri", String, "URL that the wallet uses to fetch the OpenID4VP authorization request object")
			Attribute("presentation_url", String, "Wallet deep link (openid4vp://) for QR code generation and mobile wallet support")
			Attribute("state", String, "Login state identifier used to correlate the frontend session, wallet presentation, and Hydra login flow")
			Attribute("authorize_url", String, "Hydra OAuth2 authorize URL that the browser must open to establish the Hydra session")

			Attribute("expires_in", Int, "Seconds until this login state expires")
			Required("request_uri", "presentation_url", "state", "authorize_url", "expires_in")
		})
		HTTP(func() {
			POST("/auth/login")
			Response(StatusOK)
		})
	})

	Method("loginRenew", func() {
		Description("Extends an existing OpenID4VP login state and returns an updated presentation link without changing the OIDC state.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the current login session")
			Required("state")
		})
		Result(func() {
			Attribute("request_uri", String, "URL that the wallet uses to fetch the OpenID4VP authorization request object")
			Attribute("presentation_url", String, "Wallet deep link (openid4vp://) for QR code generation and mobile wallet support")
			Attribute("state", String, "Login state identifier")
			Attribute("authorize_url", String, "Hydra OAuth2 authorize URL for this login state")
			Attribute("expires_in", Int, "Seconds until this login state expires")
			Required("request_uri", "presentation_url", "state", "authorize_url", "expires_in")
		})
		HTTP(func() {
			POST("/auth/login/renew")
			Response(StatusOK)
		})
	})

	Method("loginChallenge", func() {
		Description("Binds the Hydra login challenge from the browser authorize redirect to a pending OpenID4VP login state.")

		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the login response")
			Attribute("login_challenge", String, "Hydra login challenge from the authorize redirect to the login UI")
			Required("state", "login_challenge")
		})
		HTTP(func() {
			POST("/auth/login/challenge")
			Response(StatusNoContent)
		})
	})

	Method("consent", func() {
		Description("Handles the Hydra consent callback, accepts the consent challenge via the Admin API, and redirects the browser to continue the OAuth2 flow.")

		NoSecurity()
		Payload(func() {
			Attribute("consent_challenge", String, "Hydra consent challenge from the authorize redirect")
			Required("consent_challenge")
		})
		Result(func() {
			Attribute("location", String, "Hydra redirect_to URL after consent accept")
			Required("location")
		})
		HTTP(func() {
			GET("/auth/consent")
			Param("consent_challenge")
			Response(StatusFound, func() {
				Header("location:Location")
			})
		})
	})

	Method("callback", func() {
		Description("Handles the Hydra OIDC callback, exchanges the authorization code for tokens, and redirects to auth success.")
		NoSecurity()
		Payload(func() {
			Attribute("code", String, "Authorization code from Hydra")
			Attribute("state", String, "OIDC state parameter from Hydra")
			Attribute("error", String, "OAuth2 error code returned by Hydra when authorization fails")
			Attribute("error_description", String, "OAuth2 error description returned by Hydra when authorization fails")

		})
		Result(func() {
			Attribute("location", String, "Frontend redirect location after the token exchange")
			Required("location")
		})
		HTTP(func() {
			GET("/auth/callback")
			Param("code")
			Param("state")
			Param("error")
			Param("error_description")
			Response(StatusFound, func() {
				Header("location:Location")
			})
		})
	})

	Method("refresh", func() {
		Description("Exchanges a refresh token (from HttpOnly cookie) for a new access token.")
		NoSecurity()
		Result(func() {
			Attribute("access_token", String, "JWT access token")
			Attribute("token_type", String, "Token type (Bearer)")
			Attribute("expires_in", Int, "Token expiry in seconds")
			Required("access_token", "token_type", "expires_in")
		})
		HTTP(func() {
			POST("/auth/refresh")
			Response(StatusOK)
		})
	})

	Method("logout", func() {
		Description("Returns the Hydra OIDC logout URL for ending the current session.")
		NoSecurity()
		Result(func() {
			Attribute("logout_url", String, "Hydra OIDC logout URL")
			Required("logout_url")
		})
		HTTP(func() {
			GET("/auth/logout")
			Response(StatusOK)
		})
	})

	Method("loginStatus", func() {
		Description("Returns the current OpenID4VP login status for frontend polling.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the login response")
			Required("state")
		})
		Result(LoginStatusResult)
		HTTP(func() {
			GET("/auth/login/status")
			Param("state")
			Response(StatusOK)
		})
	})

	Method("logoutComplete", func() {
		Description("Logout callback. Clears refresh token cookie and redirects to home.")
		NoSecurity()
		Result(func() {
			Attribute("location", String, "Frontend redirect location after logout")
			Required("location")
		})
		HTTP(func() {
			GET("/auth/logout-complete")
			Response(StatusFound, func() {
				Header("location:Location")
			})
		})
	})

	Method("presentationRequest", func() {
		Description("Returns the OpenID4VP authorization request object for the wallet.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the login response")
			Required("state")
		})
		Result(PresentationRequestObject)
		HTTP(func() {
			GET("/auth/presentation/request/{state}")
			Response(StatusOK)
		})
	})

	Method("presentationCallback", func() {
		Description("Handles the wallet direct-post response and completes the Hydra login flow after presentation verification.")
		NoSecurity()
		Payload(PresentationCallbackPayload)
		Result(func() {
			Attribute("redirect_uri", String, "Redirect URI used by the frontend to continue the Hydra OIDC flow after presentation handling")
			Required("redirect_uri")
		})
		HTTP(func() {
			POST("/auth/presentation/callback")
			Response(StatusOK)
		})
	})

})

// PresentationRequestObject is the OpenID4VP authorization request returned to the wallet
// (response_uri, state, nonce, DCQL query) without a signed request JWT for now.
var PresentationRequestObject = Type("PresentationRequestObject", func() {
	Description("OpenID4VP authorization request object for the wallet.")
	Attribute("client_id", String, "OAuth2 client identifier of the DCS verifier")
	Attribute("response_type", String, "OpenID4VP response type requested from the wallet")
	Attribute("response_mode", String, "OpenID4VP response mode used by the wallet")
	Attribute("response_uri", String, "Backend endpoint where the wallet posts the presentation response")
	Attribute("state", String, "Login state identifier used to correlate the presentation response")
	Attribute("nonce", String, "Nonce that must be bound to the presentation to prevent replay")
	Attribute("dcql_query", Any, "DCQL query describing the credential format, type, and claims requested from the wallet")
	Required("client_id", "response_type", "response_mode", "response_uri", "state", "nonce", "dcql_query")
})

var PresentationCallbackPayload = Type("PresentationCallbackPayload", func() {
	Description("Wallet direct-post of a verifiable presentation.")
	Attribute("state", String, "Login state identifier from the OpenID4VP request")
	Attribute("vp_token", String, "Verifiable presentation token submitted by the wallet")
	Attribute("presentation_submission", Any, "Presentation submission metadata returned by the wallet")
	Required("state")
})

var LoginStatusResult = Type("LoginStatusResult", func() {
	Attribute("state", String, "Login state identifier")
	Attribute("status", String, "Current login status: pending, complete, failed, or expired")
	Attribute("expires_in", Int, "Number of seconds remaining before the login state expires")
	Attribute("redirect_uri", String, "Redirect URI returned when the login status is complete")
	Attribute("error_message", String, "Error message returned when the login status is failed")
	Required("state", "status", "expires_in")
})
