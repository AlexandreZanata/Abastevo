# Android account sign-in

The Account screen offers Google sign-in when a server client ID is
configured, key sign-in, sign-up with a verified recovery email and key
recovery. A person who signs in through Google for the first time confirms
their public name. The Account screen repeats the prompt until the name is
saved. The native provider may suggest a name on the device. Saving the
form sends the chosen name as the account profile name. Signed-in
users can edit their name, add or change a recovery email after a fresh
sign-in, or link Google. The forum topic form pre-fills the editable author
name from the current account profile.

Configure `googleServerClientId` as a Gradle property or
`ANPFUEL_GOOGLE_SERVER_CLIENT_ID` as an environment variable. The Android
release workflow supplies the GitHub repository variable. The value is the
OAuth Web client ID configured for the backend's native Google audience.
An empty value hides the Google option without affecting key login or email
verification. See the [operations runbook](../operations/account-sign-in.md)
for client registration and device verification.

## Dependency review

Android needs the system credential selector for Google ID tokens. These
stable releases are pinned in the version catalog and scoped to the app's
Android sign-in. The release of Google ID 1.2.1 carries Kotlin 2.4 metadata,
which the project's Kotlin 2.0.21 compiler cannot read. A Java-only
internal app library owns the Google option and token parsing. It exposes
Credential Manager types and the selected token and suggested name to
the app gateway without exposing Google ID SDK types to Kotlin.

| Google Maven artifact | Role | License declared by POM |
| --- | --- | --- |
| [`androidx.credentials:credentials:1.6.0`](https://dl.google.com/dl/android/maven2/androidx/credentials/credentials/1.6.0/credentials-1.6.0.pom) | Credential Manager request and credential-state clearing | Apache 2.0 |
| [`androidx.credentials:credentials-play-services-auth:1.6.0`](https://dl.google.com/dl/android/maven2/androidx/credentials/credentials-play-services-auth/1.6.0/credentials-play-services-auth-1.6.0.pom) | Google Play services provider for older Android versions | Apache 2.0 |
| [`com.google.android.libraries.identity.googleid:googleid:1.2.1`](https://dl.google.com/dl/android/maven2/com/google/android/libraries/identity/googleid/googleid/1.2.1/googleid-1.2.1.pom) | Google sign-in request and ID token parsing | Android Software Development Kit License |

The Credential Manager POMs specify Kotlin stdlib 2.1.20, AndroidX Core
1.15.0 and Credential Manager 1.6.0. The Play services bridge brings Google
Play authentication 21.1.1, Identity Credentials 16.0.0-alpha08, FIDO
21.0.0 and Block Store 16.4.0. Its transitive Google ID 1.1.0 resolves to the
explicit 1.2.1 dependency. The merged debug manifest keeps the Credential
Manager metadata service and selector activities non-exported. The
provider browser callback is removed from the app manifest. The app
requests a Google ID token through the user-invoked selector and
requests no access to other Google APIs. The
same Android system clearance API runs on logout and successful account
deletion to clear the credential provider's active session state.

The credential selector receives lowercase SHA-256 hex of a fresh 32-byte
base64url nonce. The app sends the raw nonce and ID token only to its
account API over HTTPS with redirects disabled. The backend verifies the
Google issuer, signature, audience, optional authorized party, expiry and
single-use nonce. Android keeps the ID token and nonce in memory during
that operation. The installed app's Keystore protects the Abastevo session
and the account key backup. See the [account security rules](../security/free-account-access.md)
and the [Android integration guide](https://developer.android.com/identity/sign-in/credential-manager-siwg-implementation).
