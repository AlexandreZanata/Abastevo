#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "monocypher/monocypher-ed25519.h"

// Small owned JNI bridge over the vendored audited Monocypher 4.0.2
// Ed25519 verifier. No cryptographic logic lives here; all math is in
// monocypher-ed25519.c (BSD-2-Clause OR CC0-1.0, see monocypher/LICENCE.md).
// Returns 1 when the 64-byte signature is valid for the 32-byte public key
// and message, 0 otherwise. Any malformed input fails closed as 0.

JNIEXPORT jint JNICALL
Java_com_anpfuel_data_map_MonocypherEd25519_verifyNative(
    JNIEnv *env,
    jclass ignored,
    jbyteArray publicKey,
    jbyteArray message,
    jbyteArray signature) {
  if (publicKey == NULL || message == NULL || signature == NULL) return 0;
  if ((*env)->GetArrayLength(env, publicKey) != 32) return 0;
  if ((*env)->GetArrayLength(env, signature) != 64) return 0;

  jsize messageLen = (*env)->GetArrayLength(env, message);
  if (messageLen < 0) return 0;

  uint8_t pub[32];
  uint8_t sig[64];
  (*env)->GetByteArrayRegion(env, publicKey, 0, 32, (jbyte *)pub);
  (*env)->GetByteArrayRegion(env, signature, 0, 64, (jbyte *)sig);
  if ((*env)->ExceptionCheck(env)) {
    (*env)->ExceptionClear(env);
    memset(pub, 0, sizeof(pub));
    memset(sig, 0, sizeof(sig));
    return 0;
  }

  uint8_t *msg = NULL;
  if (messageLen > 0) {
    // Message sizes here are manifests (bounded KiB). Stack would be unsafe;
    // use a bounded heap copy and fail closed on allocation failure.
    if (messageLen > 8 * 1024 * 1024) {
      memset(pub, 0, sizeof(pub));
      memset(sig, 0, sizeof(sig));
      return 0;
    }
    msg = (uint8_t *)malloc((size_t)messageLen);
    if (msg == NULL) {
      memset(pub, 0, sizeof(pub));
      memset(sig, 0, sizeof(sig));
      return 0;
    }
    (*env)->GetByteArrayRegion(env, message, 0, messageLen, (jbyte *)msg);
    if ((*env)->ExceptionCheck(env)) {
      (*env)->ExceptionClear(env);
      memset(pub, 0, sizeof(pub));
      memset(sig, 0, sizeof(sig));
      free(msg);
      return 0;
    }
  }

  int ok = crypto_ed25519_check(sig, pub, msg == NULL ? (uint8_t *)"" : msg, (size_t)messageLen);

  memset(pub, 0, sizeof(pub));
  memset(sig, 0, sizeof(sig));
  if (msg != NULL) {
    memset(msg, 0, (size_t)messageLen);
    free(msg);
  }
  return ok == 0 ? 1 : 0;
}
