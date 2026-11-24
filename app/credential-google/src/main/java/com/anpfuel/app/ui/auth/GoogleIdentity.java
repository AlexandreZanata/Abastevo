package com.anpfuel.app.ui.auth;

import androidx.credentials.Credential;
import androidx.credentials.CustomCredential;
import androidx.credentials.GetCredentialRequest;
import com.google.android.libraries.identity.googleid.GetSignInWithGoogleOption;
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential;
import com.google.android.libraries.identity.googleid.GoogleIdTokenParsingException;

/** Keeps Google ID SDK types off the app's Kotlin compile classpath. */
public final class GoogleIdentity {
    private GoogleIdentity() {}

    public static GetCredentialRequest buildRequest(String webClientId, String nonceClaim) {
        return new GetCredentialRequest.Builder()
                .addCredentialOption(
                        new GetSignInWithGoogleOption.Builder(webClientId)
                                .setNonce(nonceClaim)
                                .build())
                .build();
    }

    public static Selection parse(Credential credential) {
        if (!(credential instanceof CustomCredential)
                || !GoogleIdTokenCredential.TYPE_GOOGLE_ID_TOKEN_CREDENTIAL.equals(credential.getType())) {
            return null;
        }
        try {
            GoogleIdTokenCredential selected =
                    GoogleIdTokenCredential.createFrom(((CustomCredential) credential).getData());
            return new Selection(selected.getIdToken(), selected.getDisplayName());
        } catch (Exception failure) {
            // The Kotlin API does not declare its checked parsing exception.
            if (failure instanceof GoogleIdTokenParsingException) {
                return null;
            }
            throw failure;
        }
    }

    public static final class Selection {
        private final String idToken;
        private final String displayName;

        private Selection(String idToken, String displayName) {
            this.idToken = idToken;
            this.displayName = displayName;
        }

        public String getIdToken() {
            return idToken;
        }

        public String getDisplayName() {
            return displayName;
        }
    }
}
