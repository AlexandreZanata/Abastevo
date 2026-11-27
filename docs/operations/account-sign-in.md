# Account sign-in setup

## Google Auth Platform

1. Set up Branding, Audience and Data Access for Abastevo in the same Google
   Cloud project. Configure the app name, verified domain, support contact,
   consent audience and privacy-policy URL. Add test users until the consent
   screen is published and brand verification is complete.
2. Create a Web application OAuth client for the backend audience. Set its
   client ID as `ANPFUEL_GOOGLE_NATIVE_AUDIENCE` on the API and as the GitHub
   Actions repository variable `ANPFUEL_GOOGLE_SERVER_CLIENT_ID`. The Android
   release workflow passes it as `-PgoogleServerClientId` for Credential
   Manager's server client ID. A local build can use the same Gradle property
   or environment variable.
3. Create Android OAuth clients for Play Store package `com.abastevo` with the
   Play App Signing certificate SHA-1 and with the Play upload certificate
   SHA-1, and for GitHub staging package `com.anpfuel.app` (plus `.debug`
   variants such as `com.abastevo.debug` / `com.anpfuel.app.debug` using the
   debug signing certificate SHA-1). Get the Play certificate from App
   Integrity in Play Console and the local signing fingerprints with
   `./gradlew :app:signingReport`. Put the Android client IDs in the API's
   comma-separated `ANPFUEL_GOOGLE_AUTHORIZED_PARTIES` for the
   authorized-party check. Keep the configured values out of Git. Verify the
   installing channel's signing certificate when an emulator, locally signed
   APK or Play build behaves differently.

[Credential Manager setup](https://developer.android.com/identity/sign-in/credential-manager-siwg-implementation) ·
[Signing fingerprints](https://developers.google.com/android/guides/client-auth)

## SMTP delivery

Choose a transactional SMTP service, verify the sender domain and configure
SPF, DKIM and DMARC according to the provider's DNS instructions. Confirm
alignment for the configured From domain. Set a documented delivery quota
and choose an hourly Abastevo budget within it. Keep delivery records and
provider permissions limited to the verification purpose.

| API variable | Value |
|---|---|
| `ANPFUEL_MAIL_MODE` | `smtp` for actual delivery. `off` disables delivery and `memory` is for local tests. |
| `ANPFUEL_SMTP_HOST` | Provider TLS endpoint host name. |
| `ANPFUEL_SMTP_PORT` | `465` for implicit TLS or `587` for STARTTLS, unless the provider assigns another port. |
| `ANPFUEL_SMTP_TLS` | `implicit` or `starttls`. |
| `ANPFUEL_SMTP_USERNAME` | SMTP principal from the provider. |
| `ANPFUEL_SMTP_PASSWORD_FILE` | Absolute path of the password file as seen by the API. |
| `ANPFUEL_SMTP_FROM` | One authorized mailbox, such as `codes@example.invalid` in configuration examples. |
| `ANPFUEL_MAIL_MAX_CODES_PER_HOUR` | Default `200`, range `20`–`2000`. |

On the VPS, use the existing deployment secret mechanism for the API
environment and publish the SMTP password through a read-only secret volume.
Set the file path in the environment and give the API process read access to
that file. Use a volume reference for the mounted credential, with the
provider value kept outside the repository. Update the operator-managed
deployment configuration so a later rollout preserves the mount. Stage the
mount and its permissions before enabling SMTP. When API and worker share
the environment secret, either scope SMTP to the API or mount the
credential in both containers so both can validate their configuration.
In the release window,
verify migration 000302 and the generated runtime role grants. If rolling
back to the previous API image after enabling SMTP, restore
`ANPFUEL_MAIL_MODE=off` first. That image recognizes `off` and `memory`.
Leave the additive schema in place for binary rollback.

## Device smoke checks

- In a real Android build with the configured Google client, sign up using a
  dedicated authorized test mailbox. Confirm the message arrives, the code
  verifies it and the key is available through the protected backup card.
- Sign in on a second device with that key. Recover the account by email on
  the first device. Confirm the new key works, the old key is refused and
  previous sessions end on both Android and Abastevo Empresas web.
- Sign in with Google on a real phone and confirm the public name. Sign out
  and sign in again with the same Google account. Verify it reopens the
  same Abastevo account. Also exercise the email-in-use refusal and the
  fresh sign-in requirement when linking Google to an existing account.
- Confirm the public forum and comment reads show the chosen name and the
  recovery hint appears only on the signed-in profile. Use intentional
  test accounts and remove them when finished.
