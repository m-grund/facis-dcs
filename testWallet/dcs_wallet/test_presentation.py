from __future__ import annotations

import json
import unittest

import jwt
from jwt.algorithms import ECAlgorithm

from dcs_wallet.credential import decode_jwt_payload, load_credential_sd_jwt
from dcs_wallet.presentation import build_vp_token, load_jwk
from dcs_wallet.sdjwt import KB_JWT_TYP, sd_hash, split_sd_jwt


class PresentationTest(unittest.TestCase):
    def test_generated_credential_contains_issuer_header_jwk_and_holder_cnf(self) -> None:
        issuer_jwt, _, _ = split_sd_jwt(load_credential_sd_jwt("johndoe"))
        header = jwt.get_unverified_header(issuer_jwt)
        self.assertEqual(header["typ"], "dc+sd-jwt")
        self.assertIn("jwk", header)
        self.assertNotIn("kid", header)
        self.assertEqual(set(header["jwk"].keys()), {"kty", "crv", "x", "y"})

        issuer_private = load_jwk("issuer-dev.jwk")
        expected_issuer_public = {k: issuer_private[k] for k in ("kty", "crv", "x", "y")}
        self.assertEqual(header["jwk"], expected_issuer_public)

        payload = decode_jwt_payload(issuer_jwt)
        self.assertIn("cnf", payload)
        self.assertIn("jwk", payload["cnf"])
        cnf_jwk = payload["cnf"]["jwk"]
        self.assertEqual(set(cnf_jwk.keys()), {"kty", "crv", "x", "y"})

    def test_stored_credential_is_complete_sd_jwt_kb_for_debugging(self) -> None:
        issuer_jwt, disclosures, kb_jwt = split_sd_jwt(load_credential_sd_jwt("johndoe"))
        self.assertTrue(issuer_jwt.startswith("eyJ"))
        self.assertGreater(len(disclosures), 0)
        self.assertIsNotNone(kb_jwt)

        issuer_payload = decode_jwt_payload(issuer_jwt)
        self.assertIn("sub", issuer_payload)
        self.assertIn("cnf", issuer_payload)

    def test_vp_token_contains_valid_kb_jwt(self) -> None:
        vp = build_vp_token(credential_name="johndoe", nonce="unit-test-nonce", client_id="unit-test-aud")
        issuer_jwt, disclosures, kb_jwt = split_sd_jwt(vp)
        self.assertIsNotNone(kb_jwt)

        issuer_header = jwt.get_unverified_header(issuer_jwt)
        issuer_payload = jwt.decode(
            issuer_jwt,
            ECAlgorithm.from_jwk(json.dumps(issuer_header["jwk"])),
            algorithms=["ES256"],
            options={"verify_exp": False, "verify_iat": False},
        )
        cnf_jwk = issuer_payload["cnf"]["jwk"]

        kb_header = jwt.get_unverified_header(kb_jwt)
        self.assertEqual(kb_header["typ"], KB_JWT_TYP)
        kb_payload = jwt.decode(
            kb_jwt,
            ECAlgorithm.from_jwk(json.dumps(cnf_jwk)),
            algorithms=["ES256"],
            audience="unit-test-aud",
            options={"verify_iat": False},
        )
        self.assertEqual(kb_payload["nonce"], "unit-test-nonce")
        self.assertEqual(kb_payload["sd_hash"], sd_hash(issuer_jwt, disclosures))
        self.assertNotIn("sub", kb_payload)


if __name__ == "__main__":
    unittest.main()
