# Apple App Store assets — reference for a future iOS release

Native iOS work is archived (AGENTS.md); nothing here builds or submits an iOS
app. This page records the App Store Connect asset rules read on 2026-11-15 so
a future iOS release can reuse the Android material. Recheck before use.

## Screenshots [A1]

- 1–10 screenshots per device size and localization; `.jpeg`, `.jpg` or `.png`,
  **no alpha channel or transparency**.
- Provide the largest size of each class; App Store Connect scales down to the
  smaller sizes of the same class.

| Display class | Accepted portrait sizes (px; landscape swaps them) |
|---|---|
| iPhone 6.9" (Dynamic Island, large) | 1320 × 2868, 1290 × 2796, 1260 × 2736 |
| iPhone 6.5" (Face ID, large) | 1284 × 2778, 1242 × 2688 |
| iPhone 6.3" / 6.1" (Dynamic Island, medium) | 1206 × 2622, 1179 × 2556 |
| iPhone 5.5" (Home button) | 1242 × 2208 |
| iPad 13" | 2064 × 2752, 2048 × 2732 |
| iPad 12.9" (2nd gen) | 2048 × 2732 (scaled from 13" if missing) |

- Apple's page is internally inconsistent about which iPhone class is mandatory
  (the intro names the medium Dynamic Island class, the table names Face ID large
  when 6.9" is absent). Safest: upload **1320 × 2868** and **1206 × 2622**, plus
  **2064 × 2752** if the app supports iPad.
- Coming change: iPhone "Duo" sizes (outer 1398 × 2034, inner 2007 × 2853) become
  required from April 2027 for apps built with the iOS 27.1 SDK.

The Android cards in `listing-assets/` are 9:16; iPhone sizes are ~9:19.5, so a
future iOS set must be recaptured on an iOS simulator (iOS UI) and re-rendered —
the card template scales by width and can take new `size` entries in
`cards/cards.json`.

## App previews (optional) [A2]

15–30 s, ≤ 500 MB, up to 3 per size and language. H.264 (.mov/.m4v/.mp4,
10–12 Mbps) or ProRes 422 HQ, ≤ 30 fps, stereo AAC 256 kbps. iPhone Dynamic
Island/Face ID: 886 × 1920; 5.5": 1080 × 1920; iPad: 1200 × 1600. Default poster
frame at 5 s.

## App icon [A3]

1024 × 1024 square, no rounded corners (the system masks it). Supplied as an Icon
Composer file or a bitmap PNG; sRGB, Gray Gamma 2.2 or Display P3. Keep it opaque.
`docs/assets/brand/abastevo-app-icon-1024.png` already matches the size.

## Policy reminders [A4][A5][A6]

- **App Privacy** labels are required for every new app/update, including
  third-party SDK collection; a privacy policy URL is mandatory.
- **Account deletion (5.1.1(v))**: any app with account creation must offer
  deletion inside the app; a website link must go straight to the deletion page.
- **UGC (1.2)**: filtering, reporting with timely responses, blocking abusive
  users and published contact information — the same gap as Play §2.5.

## Sources

- A1 [Screenshot specifications](https://developer.apple.com/help/app-store-connect/reference/screenshot-specifications)
- A2 [App preview specifications](https://developer.apple.com/help/app-store-connect/reference/app-preview-specifications)
- A3 [HIG — App icons](https://developer.apple.com/design/human-interface-guidelines/app-icons)
- A4 [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/)
- A5 [Offering account deletion in your app](https://developer.apple.com/support/offering-account-deletion-in-your-app/)
- A6 [App privacy details](https://developer.apple.com/app-store/app-privacy-details/)
