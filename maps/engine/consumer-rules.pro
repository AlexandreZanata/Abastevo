# CoMaps JNI resolves these names dynamically. The adapter consumes an
# unpacked release AAR, so the dependency's Java/native boundary must survive R8.
# Narrow this set only after the native symbol/callback inventory is proven.
-keep,allowoptimization class app.organicmaps.sdk.** { *; }
