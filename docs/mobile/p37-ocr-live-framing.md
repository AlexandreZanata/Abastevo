# P37 price OCR: live board framing and zoom before the photo

Status: LOCAL_DONE / INTEGRATION_PENDING, 2026-11-10, branch
`android-live-board-zoom` from `android-prod-readiness` `33f3231b`. Owner
request of 2026-11-10: the camera must frame and zoom onto the price board by
itself before the photo is taken. This is the step left open by
[p37-ocr-auto-framing](p37-ocr-auto-framing.md) ("No framing or zoom before
the photo"); the post-photo framing, readings and reconciliation there are
unchanged.

No preview frame, token or label is stored, logged or committed (B-BR-OM08).

## Dependency review (AGENTS rule 11)

- **Need:** live framing needs the camera preview frames and a zoom control
  inside the app. The system camera (`ActivityResultContracts.TakePicture`)
  returns only the final file, so neither is possible without an in-app
  camera. Camera2 alone would mean writing session, rotation, focus and
  resolution handling by hand; CameraX is the Jetpack camera library.
- **Artifacts:** `androidx.camera:camera-core`, `camera-camera2`,
  `camera-lifecycle`, `camera-view`, all pinned at `1.4.2` (stable, released
  2025; `1.5.x` was not taken because it is not needed and asks for newer
  build tooling than AGP 8.7.3 / compileSdk 35). No dynamic versions.
  Transitives resolved from Google Maven: `camera-video` 1.4.2 (pulled by
  `camera-view`), `androidx.appcompat` 1.1.0 runtime floor, `concurrent-futures`,
  Guava `listenablefuture`, `auto-value-annotations`, `exifinterface` (already
  pinned at 1.3.7), `tracing`, `lifecycle-livedata`.
- **License:** CameraX is Apache-2.0 (POM `licenses`). `camera-core` also
  declares BSD-3-Clause for the bundled libyuv native code
  (`libimage_processing_util_jni.so`). Both are listed in
  `THIRD_PARTY_NOTICES.md`.
- **Security review:** AndroidX, published by Google on Google Maven and
  fetched over HTTPS. No new permission: `CAMERA` was already declared and
  requested before capture. No network access, analytics or upload in the
  library. The native library is limited by the existing `abiFilters`; the
  release artifact check (`scripts/android/artifact.py`) still requires the
  map engine in every shipped ABI.
- **Build:** the artifacts were not in the Gradle offline cache, so the first
  build after this change needs one online dependency download; later builds
  can run `--offline` again.

## Behavior contract (B-BR-PC02 / BUC-PC02 refinement: live framing)

1. **In-app camera first.** After the capture receipt is granted and
   `beginCamera` succeeds, the app opens its own full-screen camera instead of
   the system camera. It is its own full-screen window above the app's bottom
   bar. The photo (the camera's largest 4:3 size) is written to the same
   private file created by
   `PrivateCaptureFiles` and goes through the same `acceptCameraPhoto`
   check, review screen and post-photo OCR. When CameraX cannot bind (no back
   camera, camera in use, library failure), the same file is handed to the
   system camera as before. The in-app camera writes no location into the
   photo file.
2. **Live analysis finds the board.** Preview frames (about 1280 x 960) are
   read on device by the same ML Kit recognizer and `FuelBoardFraming.locate`
   rule as the post-photo path. Only the latest frame is analysed; a slow
   reading drops frames instead of queueing them. A frame is held in memory
   only while it is read; nothing from the analysis is uploaded, stored,
   logged or persisted in saved state.
3. **Zoom decision (`LiveBoardZoom`, pure).** With the board's rectangle as
   fractions of the analysed frame and the zoom ratio of that frame:
   - target ratio = current ratio x `TARGET_FILL` (0.8) / the board's larger
     side fraction, so the board fills most of the frame;
   - the zoom crops around the centre, so the ratio is also limited until the
     board's farthest edge from the centre stays inside the frame with a 5%
     margin: an off-centre board is never zoomed out of view;
   - the ratio is bounded by the device's zoom range and by `MAX_RATIO` (5x,
     the review screen's zoom limit), and never goes below the wide ratio
     (1x, or the device minimum when that is higher);
   - **hysteresis:** the ratio changes only after the board was found in two
     analyses in a row and only when the new ratio differs from the current
     one by more than 15% (a board that already fills 70% to 94% of the frame
     keeps the zoom still), or when it reaches a bound of the range;
   - **no board:** the zoom is held for six analyses without a board, then
     returns to the wide ratio so the contributor can find the board again;
   - a frame read while the zoom was still changing is not used.
4. **Manual control wins.** If the contributor pinches to zoom, the automatic
   zoom stops for the rest of that camera session; the frame overlay and hint
   keep working. Tap to focus is the library's default.
5. **Overlay and hint.** While a board is found, a rounded frame in the accent
   color is drawn around it and a pill says the board was found; otherwise a
   neutral pill asks to point at the board. The current zoom is shown as, for
   example, `2.4x`.
6. **Never captures by itself.** Only the shutter button takes the photo.
   The capture receipt window (`beginCamera` / `acceptCameraPhoto`) is
   checked exactly as before: a photo taken after the receipt's camera window
   closed is refused and its file deleted. Closing the camera (close button or
   back) deletes the pending file and behaves as a cancelled system camera.
7. **Unchanged.** The 24-hour private file handling, the review screen, the
   post-photo framing and OCR, the 15-second recognition timeout, human review
   before sending and the evidence photo (never cropped automatically).

## Validation

- Unit: `LiveBoardZoomTest` (9 cases: target ratio, device and product
  bounds, wide floor, centre limit, hysteresis, bound reach, confirmation,
  no board, invalid rectangle); `CaptureOcrViewModelTest` (a photo arriving
  after the receipt's camera window closed is refused, whatever camera
  produced it). Totals: `:domain` 935, `:application` 532, `:data` 731
  (4 skipped), `:app` 740.
- Build: `:app:assembleDebug`, `:app:assembleDebugAndroidTest`,
  `:data:assembleDebugAndroidTest`; `:app:lintDebug` reports no new error
  (the one remaining error, `home_open_map` missing translations, predates
  this change).
- Device, generated pixels only (`am instrument` on the POCO 2311DRK48G,
  API 36): `LiveBoardLocatorDeviceTest` feeds real YUV_420_888 `Image`s
  through an `ImageReader`: the board is located in an upright frame and in
  a sensor-rotated frame (rotation 90), an empty scene gives no board and a
  closed session gives none. With `OcrFormatMatrixDeviceTest`,
  `OcrFramingDeviceTest` and the synthetic `RealImageOcrDeviceTest` case:
  `OK (10 tests)`, far-board and dark-board results unchanged.
- Phone, real flow (Comunidade → contribute → station → photograph): the
  in-app camera opens full screen over the bottom bar with the 4:3 preview,
  `1,0x`, the corner guides and the "point at the board" hint; the close
  button and the back key both show "capture cancelled" and delete the
  pending file; the shutter writes a 2448 x 3264 upright JPEG (the largest
  4:3 size CameraX offers on this phone) without GPS tags, and the review
  opens with the photo attached and the post-photo OCR result. Nothing was
  sent.
- **Not verified on the phone:** the automatic zoom and the board frame on a
  real price board (the phone lay facing the ceiling; nobody aimed it at a
  board), the pinch override, the system-camera fallback, and recovery
  after process death with the camera open.

## Not done

- No accuracy measurement of the live path: the private evaluation sets are
  still photos, so they cannot exercise preview frames. The post-photo numbers
  in [p37-ocr-auto-framing](p37-ocr-auto-framing.md) remain the only
  measurement.
- No flash/torch control, no lens switching beyond what the zoom ratio
  selects on a logical multi-camera, no front camera.
