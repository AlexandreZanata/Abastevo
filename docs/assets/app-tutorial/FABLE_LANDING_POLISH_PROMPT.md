# Prompt for Fable: landing app tutorials and visual polish

Copy everything below the line into Fable. It is self-contained.

---

You are working in the Abastevo repository (`github.com/AlexandreZanata/abastevo`),
an open-source fuel-price community for Brazil: an Android app, a Go backend and
the static public site in `landing/` served at https://abastevo.com.br. Your job
is a landing-only update in three parts: put the new real app screenshots on the
site, finish every guide and tutorial with step-by-step "how to do it in the app"
sections that use them, and polish the visual quality of every section and card
without changing the design language. Then run the checks and open a pull request
to `main`.

## 1. Read first

- `AGENTS.md` (repository rules: English code/docs/commits, Conventional Commits,
  no unrelated refactors, no new dependencies without a recorded need, licence
  and security review, no progress logs or evidence dumps in the repo).
- `landing/README.md` (layout, build, rules: plain HTML/CSS with optional
  TypeScript, no framework, no runtime dependency, no CDN or remote fonts,
  essential content works without JavaScript, render untrusted data with
  `textContent`, assets content-hashed).
- `docs/product/landing/content.md` (page inventory, copy deck, claim classes,
  tokens, contrast table, amendments LH01–LH10; the latest owner amendment wins)
  and `docs/product/landing/guides.md` (editorial bounds, SEO lengths,
  structured data, breadcrumbs, accessibility).
- `docs/assets/app-tutorial/README.md`, `manifest.json` and
  `CAPTURE_CONTRACT.md` (the screenshots, their captions, capture conditions
  and the contract you must keep).
- The current site: `landing/static/index.html`, `landing/static/assets/styles.css`,
  `landing/static/como-usar/*/index.html` and `lesson.json`, `landing/src/tutorial.ts`,
  `landing/content/guias/*.md`, `landing/content/ferramentas/*.md`,
  `landing/scripts/build.mjs`, `landing/scripts/content/{layouts,markdown,site,data}.mjs`,
  `landing/static/assets/app/provenance.json` and `landing/tests/`.

Branch: start from an up-to-date `origin/main`. Per `AGENTS.md`, deliver through
`dev` synchronized from `main`; if `dev` already carries other open work, use a
branch `landing/app-tutorial-polish` from `origin/main` instead. Never push to
`main` directly, never force-push, never skip hooks or checks.

## 2. The screenshots

96 lossless captures (opaque RGB PNG, 1080 × 1920, 9:16) of the Android app,
taken on 17 Nov 2026 from commit `fd0a691283f6dc4c0970e39a4602bd8a8d248a36` on an API 36 emulator: pt-BR, light
theme, status bar fixed at 10:00, São Paulo, SP and Sorriso, MT. Masters live in
`docs/assets/app-tutorial/<feature>/NN-<slug>.png` and must never be shipped or
hot-linked; the site uses WebP derivatives.

Notes that limit their use:

- Steps marked _draft, not submitted_ show unsent forms; never say something
  was published.
- `contribute-price/05-phone-camera.png` and `06-photo-attached.png` show the
  emulator camera test scene, not a price board. Do not use them on the site;
  describe those two steps in text.
- Steps with a **Known issue** show an app defect (internal codes such as
  `FUELS` on forum chips, "Enviar 1 preços"). Prefer text for those steps until
  the app is fixed and recaptured.
- `textOnlySteps` (account creation and deletion screens, the forum report and
  block menu, company mode, private quality reports) were not captured because
  the app protects them with `FLAG_SECURE` or staging had no content for them.
  Describe them in text with the exact app labels; never mock them up.
- Station feedback (ratings and comments on a station) is behind a feature flag
  in this build; present it as "em breve" or leave it out, never as available.

Every image, in tutorial order (caption in English; write the site copy in
pt-BR):

**First open: terms and welcome** (`first-open/`) — Accept the Terms of Use and Privacy Policy, read the welcome pages and choose how to find the city.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/first-open/01-terms-gate.png` | On first open the app shows the Terms of Use and Privacy Policy before anything else. |  |
| `docs/assets/app-tutorial/first-open/02-accept-terms.png` | Tick the acceptance box; "Aceitar e continuar" only works after a real tick. |  |
| `docs/assets/app-tutorial/first-open/03-welcome.png` | Welcome page: the app shows the weekly ANP survey prices. |  |
| `docs/assets/app-tutorial/first-open/04-works-offline.png` | After the first sync the averages work without internet. |  |
| `docs/assets/app-tutorial/first-open/05-community-rules.png` | Contribution rules: photograph only public price boards and never invent prices. |  |
| `docs/assets/app-tutorial/first-open/06-choose-survey-week.png` | Choose which ANP survey week to download now, or skip and sync later. |  |
| `docs/assets/app-tutorial/first-open/07-find-city.png` | Find the city once with the device location, or choose it manually. |  |

**Choose the city** (`choose-city/`) — Pick a state and a municipality; the choice drives prices, stations and the map.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/choose-city/01-state-list.png` | The location picker lists the states; "Usar localização atual" detects the city instead. | sao-paulo-sp |
| `docs/assets/app-tutorial/choose-city/02-search-state.png` | Type part of the state name to filter the list. | sao-paulo-sp |
| `docs/assets/app-tutorial/choose-city/03-municipality-list.png` | After choosing the state, the municipalities appear in alphabetical sections. | sao-paulo-sp |
| `docs/assets/app-tutorial/choose-city/04-search-municipality.png` | Search the municipality by name and tap it. | sao-paulo-sp |
| `docs/assets/app-tutorial/choose-city/05-home-with-city.png` | The home screen now shows the city and the active survey week at the top. | sao-paulo-sp |

**Fuel prices in the city** (`fuel-prices/`) — Read the ANP weekly average per fuel, sort the cards and switch to community prices.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/fuel-prices/01-anp-average-cards.png` | Each card shows the ANP weekly average for a fuel, the number of surveyed stations and the source badge. | sao-paulo-sp |
| `docs/assets/app-tutorial/fuel-prices/02-more-fuels.png` | Scroll for diesel S500/S10, GNV and GLP (13 kg). | sao-paulo-sp |
| `docs/assets/app-tutorial/fuel-prices/03-home-shortcuts.png` | Shortcuts at the end of the list open the map, location, history, stations, vehicles and settings. | sao-paulo-sp |
| `docs/assets/app-tutorial/fuel-prices/04-sort-prices.png` | "Filtros" lets you order prices by average, lowest or highest. | sao-paulo-sp |
| `docs/assets/app-tutorial/fuel-prices/05-lowest-price.png` | With "Menor preço" each card shows the lowest surveyed price for that fuel. | sao-paulo-sp |
| `docs/assets/app-tutorial/fuel-prices/06-community-prices.png` | The "Comunidade" toggle shows recent prices sent by drivers, kept apart from ANP data. | sorriso-mt |

**Stations, station page and route** (`stations/`) — List surveyed stations for a fuel, search by name, open the station page, check ANP and community prices and start a route.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/stations/01-stations-for-fuel.png` | Tapping a fuel card opens the surveyed stations for that fuel, cheapest first. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/02-station-cards.png` | Each station card shows the price, the brand, the public ANP address and the collection date. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/03-choose-fuel.png` | Change the fuel from the selector at the top of the list. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/04-search-by-name.png` | Type part of a station name to filter the list. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/05-station-profile.png` | The station page shows the name, brand, address and the main actions. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/06-community-and-anp-prices.png` | Choose a fuel chip; community prices (beta) and the ANP reference appear in separate cards. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/07-anp-reference-price.png` | The ANP reference price always shows the collection date and the survey week. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/08-fuel-quality.png` | "Consultar análises de qualidade" lists ANP quality sampling for the station, with its limits. | sao-paulo-sp |
| `docs/assets/app-tutorial/stations/09-allow-location-for-route.png` | "Rota e ir" asks for the device location the first time (Android system dialog). | sao-paulo-sp; Android system screen |
| `docs/assets/app-tutorial/stations/10-driver-experience.png` | Ratings and comments live in "Experiência dos motoristas"; this build shows the verification notice because station feedback is behind a feature flag. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/stations/11-suggest-correction.png` | "Sugerir correção" sends a name, address or permanent-closure correction for review. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/stations/12-represent-your-station.png` | "Entender como funciona" explains how a business can represent its station; it is not available in the beta. | sorriso-mt; signed in |

**Offline map download** (`map-offline/`) — Download the offline map of the region so the map and routes work without internet.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/map-offline/01-download-suggestion.png` | Opening the map of a city without a pack offers "Baixar mapa de Mato Grosso"; tap "Baixar". | sorriso-mt |
| `docs/assets/app-tutorial/map-offline/02-download-progress.png` | Downloads shows the total progress and each package (region, world base map, coastlines); Wi-Fi only by default. | sorriso-mt |
| `docs/assets/app-tutorial/map-offline/03-offline-pack-active.png` | When it finishes, the region pack is listed as active. | sorriso-mt |
| `docs/assets/app-tutorial/map-offline/04-offline-map-ready.png` | Back on the map, streets and stations render from the offline pack. | sorriso-mt |

**Map, station sheet and navigation** (`map/`) — Explore stations on the map, filter by source and service, open a station, save it and navigate with turn-by-turn guidance.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/map/01-stations-on-map.png` | The map shows fuel stations (orange) and EV chargers (teal) of the chosen city. | sorriso-mt |
| `docs/assets/app-tutorial/map/02-map-filters.png` | "Filtros" chooses the city, the price source (ANP or community), the service (fuel or charging) and the product. | sorriso-mt |
| `docs/assets/app-tutorial/map/03-station-sheet.png` | Tap a station to see its price with source, survey week and collection date. | sorriso-mt |
| `docs/assets/app-tutorial/map/04-save-favorite.png` | "Salvar nos favoritos" keeps the station on this device. | sorriso-mt |
| `docs/assets/app-tutorial/map/05-route-preview.png` | "Rota e ir" draws the route from the current position with time and distance. | sorriso-mt |
| `docs/assets/app-tutorial/map/06-route-maneuvers.png` | "Manobras" lists every turn before you leave. | sorriso-mt |
| `docs/assets/app-tutorial/map/07-turn-by-turn-navigation.png` | "Iniciar navegação" starts offline turn-by-turn guidance with voice, zoom and recentre controls. | sorriso-mt |
| `docs/assets/app-tutorial/map/08-ev-charger-sheet.png` | EV chargers come from open sources; the sheet says ANP has no electricity tariffs. | sorriso-mt |

**ANP survey weeks offline** (`survey-weeks/`) — Download earlier ANP survey weeks to read them offline and switch the active week.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/survey-weeks/01-open-week-picker.png` | Tap the week chip at the top to open "Semana de pesquisa". | sorriso-mt |
| `docs/assets/app-tutorial/survey-weeks/02-week-list.png` | Each week shows when it was updated and whether it is downloaded; "Baixar" saves it offline. | sorriso-mt |
| `docs/assets/app-tutorial/survey-weeks/03-downloading-week.png` | The download imports the ANP data in place and can be cancelled. | sorriso-mt |
| `docs/assets/app-tutorial/survey-weeks/04-week-downloaded.png` | Downloaded weeks are marked "Baixada". | sorriso-mt |
| `docs/assets/app-tutorial/survey-weeks/05-older-week-active.png` | Tapping a downloaded week makes it active: the chip and the ANP reference line change. | sorriso-mt |

**Price history** (`history/`) — Compare the ANP average of a fuel across the downloaded weeks.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/history/01-weekly-history.png` | "Histórico" charts the city average for each downloaded week, newest first. | sorriso-mt |
| `docs/assets/app-tutorial/history/02-choose-fuel.png` | Change the fuel from the selector. | sorriso-mt |
| `docs/assets/app-tutorial/history/03-diesel-s10-trend.png` | Diesel S10 history for the same weeks. | sorriso-mt |
| `docs/assets/app-tutorial/history/04-tap-a-week.png` | Tap a bar to read that week's value. | sorriso-mt |

**Vehicles, tank-fill cost and fuel log** (`vehicles/`) — Register up to three vehicles, see the cost to fill the tank, turn on price-drop alerts and log fill-ups.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/vehicles/01-no-vehicles.png` | "Veículos" starts empty; the + button adds a vehicle. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/02-name-and-tank.png` | Give the vehicle a name, choose the propulsion and type the tank capacity in litres. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/03-fuel-and-price-alert.png` | Choose the fuel and the price reference, and switch on "Avisar quando o preço cair" for a local price-drop alert. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/04-vehicle-saved.png` | The saved vehicle lists its fuel, tank size and price reference. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/05-tank-fill-cost.png` | The home screen shows the cost to fill the tank at the cheapest surveyed station and a shortcut to go there. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/06-fuel-log.png` | "Lançamentos" keeps the vehicle's fill-ups and expenses on the device. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/07-add-fill-up.png` | Record litres, full or partial tank, total paid and odometer. | sorriso-mt |
| `docs/assets/app-tutorial/vehicles/08-fuel-log-entry.png` | The entry appears in the log, stored locally. | sorriso-mt |

**Settings** (`settings/`) — Sync, automatic downloads, language, search distance, data retention, storage and account links.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/settings/01-sync-and-preferences.png` | Sync now, change the survey week and choose background sync and Wi-Fi-only rules. | sorriso-mt |
| `docs/assets/app-tutorial/settings/02-language-and-search-distance.png` | Pick the app language and the radius of "Mais barato perto de mim". | sorriso-mt |
| `docs/assets/app-tutorial/settings/03-data-retention-and-city-directory.png` | Choose how many weeks of station data to keep and whether to keep the city's public directory offline. | sorriso-mt |
| `docs/assets/app-tutorial/settings/04-storage-account-and-about.png` | See storage use, clear caches, open account and privacy, share the app and the official ANP page. | sorriso-mt |

**Profile and tools** (`profile-and-tools/`) — The Profile tab groups account, tools, references, Premium status and project contributors.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/profile-and-tools/01-tools-list.png` | "Ferramentas e referências": games, company mode, vehicles, settings and contributions. | sorriso-mt |
| `docs/assets/app-tutorial/profile-and-tools/02-more-tools.png` | More tools: ANP history, surveyed stations, survey week, help and privacy. | sorriso-mt |
| `docs/assets/app-tutorial/profile-and-tools/03-premium-status.png` | Premium shows the plan status; paid purchase is disabled in this version and never affects trust. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/profile-and-tools/04-project-contributors.png` | "Contribuidores do projeto" lists approved contributor profiles. | sorriso-mt; signed in |

**Help, rules and privacy** (`help-and-privacy/`) — Community rules, reporting and appeals, privacy summary and the terms accepted on this device.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/help-and-privacy/01-help-and-community-rules.png` | "Ajuda e regras da comunidade": how to contribute well, community rules, reports and appeals. | sorriso-mt |
| `docs/assets/app-tutorial/help-and-privacy/02-privacy-and-terms.png` | "Privacidade": full policy, web account deletion and the terms version accepted on this device. | sorriso-mt |

**Free account: create and delete** (`account/`) — Create a free account with only a username, see the signed-in profile and delete the account.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/account/01-guest-profile.png` | As a guest, Profile offers "Entrar ou criar conta gratuita"; browsing never needs an account. | sorriso-mt |
| `docs/assets/app-tutorial/account/02-signed-in-profile.png` | After creating the account (username only, no email or password) Profile shows "Conectado". | sorriso-mt; signed in |
| `docs/assets/app-tutorial/account/03-after-account-deletion.png` | After "Gerenciar conta → Excluir minha conta → Sim, excluir" the app returns to guest mode. | sorriso-mt |

**Driver hub** (`driver-hub/`) — Quick fuel and charging expenses, shifts and earnings for app drivers.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/driver-hub/01-driver-hub.png` | "Central do motorista" shows the vehicle in use, quick expense entries and work tracking (Premium gate shown). | sorriso-mt; signed in |
| `docs/assets/app-tutorial/driver-hub/02-shift-and-earnings.png` | "Turno e ganhos": start a shift with the odometer and record the day's net earnings. **Known issue:** Shift and earnings labels are hard-coded Portuguese in DriverWorkScreen.kt (not string resources). | sorriso-mt; signed in |

**Community tab** (`community/`) — Entry point for contributing prices, the forum and the community price feed.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/community/01-community-tab.png` | "Comunidade" opens the price contribution, the Abastevo forum and the community price feed. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/community/02-community-prices-empty.png` | The feed filters by order and fuel; with no recent community price it invites a contribution. | sorriso-mt; signed in |

**Forum: read, comment, post and block** (`forum/`) — Read editorial posts, filter by category, react and comment, write a topic and manage blocked users.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/forum/01-forum-feed.png` | "Fórum Abastevo" lists posts with author, category and time; the pencil starts a new topic. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/forum/02-filter-by-category.png` | Category tabs filter the feed (fuels, e-mobility, traffic law…). | sorriso-mt; signed in |
| `docs/assets/app-tutorial/forum/03-read-article.png` | Open a post to read it in full. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/forum/04-reactions-and-comments.png` | At the end: comment, like, dislike, share and the live comment list. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/forum/05-write-comment.png` | Write a comment of up to 280 characters and tap "Publicar" (new comments go through moderation). | sorriso-mt; signed in; draft, not submitted |
| `docs/assets/app-tutorial/forum/06-new-topic-draft.png` | "Novo tópico": title, category and content. **Known issue:** Category chips show internal codes (FUELS, ELECTRIC, TRAFFIC_RULES) instead of translated labels. | sorriso-mt; signed in; draft, not submitted |
| `docs/assets/app-tutorial/forum/07-new-topic-options.png` | Optional state, IBGE municipality, fuel and public name, then "Publicar tópico". **Known issue:** Fuel chips show internal codes (GASOLINE_REGULAR, GASOLINE_PREMIUM). | sorriso-mt; signed in; draft, not submitted |
| `docs/assets/app-tutorial/forum/08-blocked-users.png` | "Gerenciar conta → Usuários bloqueados" lists blocked people; they are not notified and can be unblocked any time. | sorriso-mt; signed in |

**Contribute a price with a photo** (`contribute-price/`) — Choose the station, photograph the price board, review the reading or type the price, and send it to the community queue.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/contribute-price/01-choose-station.png` | "Colaborar com preço" first asks which station the price belongs to. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/contribute-price/02-search-station.png` | Search the station by name. | sorriso-mt; signed in |
| `docs/assets/app-tutorial/contribute-price/03-station-selected.png` | With the station selected, tap "Fotografar preço". | sorriso-mt; signed in |
| `docs/assets/app-tutorial/contribute-price/04-allow-camera.png` | Android asks for camera permission the first time. | sorriso-mt; signed in; Android system screen |
| `docs/assets/app-tutorial/contribute-price/05-phone-camera.png` | The phone camera opens; photograph only the price board. (Emulator test scene — not a price board.) | sorriso-mt; signed in; Android system screen; emulator test scene: do not use |
| `docs/assets/app-tutorial/contribute-price/06-photo-attached.png` | The photo is attached; crop to the frame or read it again. With no readable board the app asks you to type the prices. | sorriso-mt; signed in; emulator test scene: do not use |
| `docs/assets/app-tutorial/contribute-price/07-type-price-and-send.png` | Add the fuel, type the price (R$ 6,25) and tap "Enviar"; blank fuels are not sent. **Known issue:** Send button reads "Enviar 1 preços" (plural not handled). | sorriso-mt; signed in; draft, not submitted |
| `docs/assets/app-tutorial/contribute-price/08-my-contributions.png` | "Minhas contribuições" tracks sends, queue and cancellations on this device only. | sorriso-mt; signed in |

**Offline games** (`games/`) — Two offline mini-games with local records only.

| Image | Caption | Context |
|---|---|---|
| `docs/assets/app-tutorial/games/01-games-hub.png` | "Games": play offline, even without an account; records stay on the device. | sorriso-mt |
| `docs/assets/app-tutorial/games/02-second-game.png` | Fuel Run and accessibility options: mute, reduce motion, save battery. | sorriso-mt |
| `docs/assets/app-tutorial/games/03-how-to-play.png` | Each game explains how to play before "Começar". | sorriso-mt |
| `docs/assets/app-tutorial/games/04-match-over.png` | End of a match with the score and the local record. | sorriso-mt |

Text-only steps:

- `account`, after `01-guest-profile.png`: Type a username (3–20 letters and numbers, starting with a letter) and tap "Criar conta". The account screens are protected with FLAG_SECURE so the account key never appears in screenshots.
- `account`, after `02-signed-in-profile.png`: To delete: Profile → "Gerenciar conta" → "Excluir minha conta" → "Sim, excluir". This screen is FLAG_SECURE; the web path is /excluir-conta/.
- `forum`, after `07-new-topic-options.png`: On a topic or comment written by someone else, the ⋮ menu offers "Denunciar tópico", "Denunciar comentário", "Denunciar usuário" and "Bloquear usuário". Staging had only official editorial posts, which do not show the menu, so these dialogs were not captured.
- `profile-and-tools`, after `04-project-contributors.png`: "Modo empresa", "Entrar em uma empresa" and "Meus relatos privados" are FLAG_SECURE screens and are not captured.

## 3. Task (a): replace the old app screenshots

The home section `#aplicativo` ("O aplicativo em telas reais",
`landing/static/index.html`) still says "Capturas autênticas do app em
desenvolvimento (3.1.0)" and shows three old 1220 × 2712 captures
(`explore-dark`, `station-sources`, `community-feed`).

- Replace them with a curated set of 4–6 new captures that tell the product
  story in order. Suggested: `fuel-prices/01-anp-average-cards.png`,
  `stations/06-community-and-anp-prices.png`, `map/05-route-preview.png`,
  `vehicles/05-tank-fill-cost.png`, `survey-weeks/02-week-list.png`,
  `forum/01-forum-feed.png`.
- Remove the version number. Use factual wording such as "Capturas reais do
  aplicativo Android em desenvolvimento, feitas em 17 de novembro de 2026.
  Valores exibidos são referência ANP datada, nunca preço vivo na bomba."
- Derivatives: generate WebP at 360, 720 and 1080 px wide with the parameters
  already recorded in `landing/static/assets/app/provenance.json` (Pillow,
  quality 85, method 6, LANCZOS, downscale only, no crop or retouch), named
  `<feature>-<NN-slug>-<width>w.webp` (for example
  `map-05-route-preview-720w.webp`) in `landing/static/assets/app/`. Record
  every output in `provenance.json` with its master path under
  `docs/assets/app-tutorial/`, width, height, bytes and SHA-256. Keep the stem:
  contract rule C3 relies on it.
- Update `width`/`height` attributes and the phone frame for 9:16 (360 × 640)
  so there is no layout shift. Write pt-BR `alt` text that describes what the
  screen shows, including city and week when visible.
- Remove old derivatives and provenance entries only when no page or test
  references them any more; update the tests that pin them.

## 4. Task (b): finish every guide and tutorial with "how to do it in the app"

Use the captions and the step order above; quote every app control with its
exact pt-BR label as it appears in the screenshot (contract rule C3).

Tutorial hub and chapters (`/como-usar/` and `landing/static/como-usar/<slug>/`):
replace each chapter's old images with the new captures and add the missing
steps. Suggested mapping:

| Chapter | Captures |
|---|---|
| `escolher-cidade` | `first-open/07`, `choose-city/01–05` |
| `precos-anp` | `fuel-prices/01–05` |
| `semanas-anp` | `first-open/06`, `survey-weeks/01–05` |
| `postos` | `stations/01–04`, `map/01–03` |
| `precos-do-posto` | `stations/05–07`, `fuel-prices/06` |
| `perfil-do-posto` | `stations/05`, `08`, `10–12` |
| `historico` | `history/01–04` |
| `veiculos` | `vehicles/01–08` |
| `offline-e-recuperacao` | `map-offline/01–04`, `map/05–07`, `survey-weeks/02–04` |
| `fotografar-preco` | `contribute-price/01–04`, `07` (text for 05–06) |
| `minhas-contribuicoes` | `contribute-price/08` |
| `comunidade` | `community/01–02`, `forum/01–05` |
| `denunciar-abuso` | `forum/08` plus the text-only report/block step |
| `conta-e-privacidade` | `account/01–03`, `help-and-privacy/02`, `first-open/01–02` |
| `configuracoes` | `settings/01–04` |
| `ajuda-e-regras` | `help-and-privacy/01`, `first-open/05` |

For each `lesson.json` step: `image` (the 720w derivative), `imageSha256` (of
that derivative), `nativeWidth` 1080, `nativeHeight` 1920, a `target` box
measured on the 1080 × 1920 master around the control the step talks about,
`sourceFiles` from the manifest step `sources`, `capturedAt` "2026-11-17",
`appVersion` "3.1.0-debug", `appSourceSha` "fd0a691283f6dc4c0970e39a4602bd8a8d248a36", `privacyReviewed` true,
`demoOnly` false, and pt-BR `instructionPtBr`, `controlLabelPtBr`,
`expectedPtBr`, `recoveryPtBr`, `altPtBr`. Keep `landing/src/tutorial.ts`
validation passing; extend it only if a new field is truly needed. Keep chapter
slugs, numbering, availability labels and the hub's step counts accurate.

Guides (`landing/content/guias/*.md`): add a section "Como fazer no aplicativo"
with 2–5 numbered steps and the matching screenshots, written for that guide's
question. Suggested pairing:

| Guide | Captures |
|---|---|
| `como-economizar-combustivel` | `fuel-prices/04–05`, `stations/01`, `map/05`, `vehicles/05` |
| `como-calcular-consumo-medio` | `vehicles/06–08` |
| `precos-diferentes-entre-cidades` | `choose-city/01–05`, `history/01` |
| `gasolina-comum-ou-aditivada` | `fuel-prices/01`, `stations/03` |
| `diesel-s10-ou-s500` | `fuel-prices/02`, `history/03` |
| `gnv-compensa` | `fuel-prices/02`, `vehicles/02` |
| `icms-sobre-combustiveis` | `history/01`, `survey-weeks/05` |
| `como-e-formado-o-preco-da-gasolina` | `stations/07`, `fuel-prices/01` |
| `combustivel-adulterado` | `stations/08`, `help-and-privacy/01` |
| `direitos-do-consumidor-no-posto` | `stations/11`, `contribute-price/01–03`, `help-and-privacy/01` |

Tools (`/ferramentas/` and `landing/content/ferramentas/*.md`): add a short
"No aplicativo" block to each calculator page: `encher-o-tanque` →
`vehicles/02`, `03`, `05`; `consumo-medio` → `vehicles/06–08`; `custo-por-km`
and `custo-de-viagem` → `map/05`, `vehicles/05`; `etanol-ou-gasolina` →
`fuel-prices/01`, `05` and `forum/03` (the app's own editorial on the topic).

If the Markdown renderer cannot express figures with captions, add the smallest
safe extension to `landing/scripts/content/markdown.mjs` (escaped output, alt
text required, explicit width/height, lazy loading) with tests; never pass raw
HTML through.

## 5. Task (c): visual refinement without changing the style

Polish every section and card so the site looks finished and professional:
home (hero, "Explore o que dá para fazer" cards, "Feito para quem abastece toda
semana" audience block, "Dados abertos" open-data band and its links to prices
by state, popular guides and tools entry points, "O aplicativo em telas reais"
gallery, "Perguntas frequentes", footer), the `/ferramentas/` hub and calculator
cards, guide hubs and guide pages, and the `/como-usar/` hub and chapters.

Allowed: spacing rhythm on the existing 4 px scale, consistent card padding,
radii, borders and image frames, alignment and grid fixes, heading/body
hierarchy within the existing type scale, balanced line lengths, hover, focus
and active states, consistent icon sizes, better image framing for 9:16
captures, responsive fixes at 320, 390, 768, 1024 and 1440 px, removing
horizontal overflow, `prefers-reduced-motion`, and contrast that stays at WCAG
AA or better (recompute any pair you touch).

Not allowed: changing the visual style or brand (black header/footer, white
surfaces, existing colour tokens, Manrope and Source Sans 3, logo, icons and
illustrations), the content model or page inventory, URLs, slugs, redirects,
canonical tags, titles, meta descriptions, JSON-LD, sitemap rules, store state
logic, analytics posture (none), core behaviour of search and calculators,
or adding a dependency, framework, CDN, remote font, tracker or inline script.

## 6. Editorial and policy constraints

- pt-BR copy, factual and calm. No superlatives or claims such as "melhor",
  "nº 1", "o mais barato garantido", "grátis" as a hook, savings figures,
  coverage or user counts, rankings, or calls to download that imply the app
  is on Google Play (the store state stays prelaunch: "Em breve no Google
  Play"). Follow the claim classes in `docs/product/landing/content.md`.
- ANP data is always labelled with its source and survey date ("Referência ANP
  • 8 a 14 de nov. de 2026"); never present it as the pump price now.
- Community data is always shown and described apart from official ANP data;
  payment never buys trust.
- No personal data, contributor locations or production photos; the captures
  already follow this, keep alt text and captions free of personal data too.
- Not affiliated with the ANP; keep the existing disclaimers.

## 7. Checks

Run and fix until all pass:

```bash
cd landing && npm ci && npm run typecheck && npm test && npm run build && npm run check
```

```bash
make quick-verify
```

```bash
python3 scripts/app_tutorial/tutorial_contract.py validate
```

```bash
python3 scripts/app_tutorial/tutorial_contract.py stale --strict
```

Also `git diff --check` and `bash scripts/scan-secrets.sh`. Review the built
pages in a browser at 390 and 1440 px (home, one chapter, one guide, one tool)
with and without JavaScript, and check keyboard focus order.

## 8. Pull request

Open one pull request to `main` with Conventional Commits (for example
`feat(landing): use 17 Nov app captures in gallery and tutorials`,
`feat(landing): add in-app steps to guides and tools`,
`style(landing): refine section and card polish`). The description lists what
changed per page, the images used, the check results and any app defect you
worked around. Do not merge it; leave it for review.

## Done when

- The `#aplicativo` gallery shows the new captures, with no "3.1.0" and no old
  1220 × 2712 images referenced by any page.
- Every `/como-usar/` chapter, every guide and every calculator page has
  accurate step-by-step "how to do it in the app" content in pt-BR with the new
  screenshots or explicit text-only steps, quoting exact app labels.
- `provenance.json` records every new derivative; derivative names keep the
  capture stems, so `tutorial_contract.py stale` reports their pages.
- The visual polish is applied across the listed sections without any change
  to style, brand, colours, content model, URLs, SEO or structured data.
- All checks in section 7 pass and the pull request to `main` is open.
