# Home map action localization

Status: LOCAL_DONE
Validation: PASS

2026-11-11, above enterprise head ec92ab38. The owner requested fixing the
aggregate app lint error for home_open_map. Added the missing German, Spanish,
French, Japanese, Russian and Simplified Chinese translations, preserving
the existing English and Brazilian Portuguese values.

The complete :app:lintDebug invocation passed offline with the existing
JDK17/SDK35: zero errors, 347 existing warnings. No suppression, baseline,
translation setting or lint threshold was changed. XML parsing and key
uniqueness passed for all eight resource files. Other checkouts were not
modified. Delivery is INTEGRATION_PENDING through PR202; this does not
resolve the separate domain coverage or enterprise activation obligations.
