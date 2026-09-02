# Security

Password verification uses Argon2id PHC hashes; plaintext is neither logged nor accepted on the hash command line. Sessions are AEAD cookies with HttpOnly, Secure and SameSite=Lax attributes. Rate limiting is per observed client IP. API measurement endpoints recheck authentication and run ownership. Forwarded IP headers are accepted only from configured proxy CIDRs. Security headers disable framing, sniffing, referrers and caching. OIDC is configured through discovery with state, nonce and PKCE; deployment must supply an issuer, client credentials and normalized email allowlist.
