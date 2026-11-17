# Android app tutorial captures

Step-by-step screenshots of every Abastevo Android feature, taken on an
emulator for the public tutorials (`/como-usar/`, guides and tools pages on
`landing/`). Each folder is one feature; files are numbered in tutorial order
(`NN-step-slug.png`).

| File | Purpose |
|---|---|
| [`manifest.json`](manifest.json) | Machine-readable contract: capture conditions, every step with caption, the string resources and Compose sources that draw it, SHA-256 and landing pages |
| [`CAPTURE_CONTRACT.md`](CAPTURE_CONTRACT.md) | The rules and the step-by-step procedure to recapture a screen when the app changes and to update the site page or field that shows it |
| [`FABLE_LANDING_POLISH_PROMPT.md`](FABLE_LANDING_POLISH_PROMPT.md) | Self-contained prompt for the landing update that uses these captures |
| `<feature>/NN-*.png` | Lossless captures, opaque RGB, 1080 × 1920 |

## Capture conditions (summary)

Debug build of the commit in `manifest.json` (`capture.appSourceCommit`),
talking to `https://abastevo.com.br`, on a dedicated API 36 emulator
(`google_apis` x86_64, Pixel 7 profile): system and app locale pt-BR, time zone
America/Sao_Paulo, light theme, System UI demo mode (10:00, full battery,
Wi-Fi only), `wm size 1080x1920` at 420 dpi, on-screen keyboard disabled.
Cities: São Paulo, SP and Sorriso, MT (map and route use Sorriso, where
stations are spread out). Full list and the reproduction commands:
[CAPTURE_CONTRACT.md](CAPTURE_CONTRACT.md).

## Privacy and data

- Prices, station names and addresses are public ANP records for the survey
  week 8–14 Nov 2026 (plus three earlier weeks downloaded in the app).
- Locations are synthetic emulator fixes on public city-centre points; no
  contributor GPS appears.
- Signed-in steps used a throwaway username-only account that was deleted
  through the app at the end. Nothing was submitted to the community, forum or
  corrections queue: comment, topic and price screens are unsent drafts.
- Screens protected with `FLAG_SECURE` (account key, account management,
  company mode, private quality reports) are never captured; their steps are
  described in text below.
- `contribute-price/05` and `06` show the emulator's built-in camera test
  scene, not a price board. These images are not OCR training data and must
  never be used as such.

## Landing derivatives

The site must not hot-link these masters. Generate WebP derivatives that keep
the stem, e.g. `forum/05-write-comment.png` →
`forum-05-write-comment-720w.webp`, so `tutorial_contract.py stale` can find
every page that shows a step (see the contract, rule C3).

## Steps

Generated from `manifest.json` by
`python3 scripts/app_tutorial/tutorial_contract.py render-readme --write`; do
not edit this section by hand.

<!-- BEGIN GENERATED STEPS: tutorial_contract.py render-readme -->

### First open: terms and welcome

Accept the Terms of Use and Privacy Policy, read the welcome pages and choose how to find the city.

Landing pages: `/como-usar/`, `/como-usar/conta-e-privacidade/`, `/termos/`, `/privacidade/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`first-open/01-terms-gate.png`](first-open/01-terms-gate.png) | On first open the app shows the Terms of Use and Privacy Policy before anything else. |
| 02 | [`first-open/02-accept-terms.png`](first-open/02-accept-terms.png) | Tick the acceptance box; "Aceitar e continuar" only works after a real tick. |
| 03 | [`first-open/03-welcome.png`](first-open/03-welcome.png) | Welcome page: the app shows the weekly ANP survey prices. |
| 04 | [`first-open/04-works-offline.png`](first-open/04-works-offline.png) | After the first sync the averages work without internet. |
| 05 | [`first-open/05-community-rules.png`](first-open/05-community-rules.png) | Contribution rules: photograph only public price boards and never invent prices. |
| 06 | [`first-open/06-choose-survey-week.png`](first-open/06-choose-survey-week.png) | Choose which ANP survey week to download now, or skip and sync later. |
| 07 | [`first-open/07-find-city.png`](first-open/07-find-city.png) | Find the city once with the device location, or choose it manually. |

### Choose the city

Pick a state and a municipality; the choice drives prices, stations and the map.

Landing pages: `/como-usar/escolher-cidade/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`choose-city/01-state-list.png`](choose-city/01-state-list.png) | The location picker lists the states; "Usar localização atual" detects the city instead. _(sao-paulo-sp)_ |
| 02 | [`choose-city/02-search-state.png`](choose-city/02-search-state.png) | Type part of the state name to filter the list. _(sao-paulo-sp)_ |
| 03 | [`choose-city/03-municipality-list.png`](choose-city/03-municipality-list.png) | After choosing the state, the municipalities appear in alphabetical sections. _(sao-paulo-sp)_ |
| 04 | [`choose-city/04-search-municipality.png`](choose-city/04-search-municipality.png) | Search the municipality by name and tap it. _(sao-paulo-sp)_ |
| 05 | [`choose-city/05-home-with-city.png`](choose-city/05-home-with-city.png) | The home screen now shows the city and the active survey week at the top. _(sao-paulo-sp)_ |

### Fuel prices in the city

Read the ANP weekly average per fuel, sort the cards and switch to community prices.

Landing pages: `/como-usar/precos-anp/`, `/guias/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`fuel-prices/01-anp-average-cards.png`](fuel-prices/01-anp-average-cards.png) | Each card shows the ANP weekly average for a fuel, the number of surveyed stations and the source badge. _(sao-paulo-sp)_ |
| 02 | [`fuel-prices/02-more-fuels.png`](fuel-prices/02-more-fuels.png) | Scroll for diesel S500/S10, GNV and GLP (13 kg). _(sao-paulo-sp)_ |
| 03 | [`fuel-prices/03-home-shortcuts.png`](fuel-prices/03-home-shortcuts.png) | Shortcuts at the end of the list open the map, location, history, stations, vehicles and settings. _(sao-paulo-sp)_ |
| 04 | [`fuel-prices/04-sort-prices.png`](fuel-prices/04-sort-prices.png) | "Filtros" lets you order prices by average, lowest or highest. _(sao-paulo-sp)_ |
| 05 | [`fuel-prices/05-lowest-price.png`](fuel-prices/05-lowest-price.png) | With "Menor preço" each card shows the lowest surveyed price for that fuel. _(sao-paulo-sp)_ |
| 06 | [`fuel-prices/06-community-prices.png`](fuel-prices/06-community-prices.png) | The "Comunidade" toggle shows recent prices sent by drivers, kept apart from ANP data. _(sorriso-mt)_ |

### Stations, station page and route

List surveyed stations for a fuel, search by name, open the station page, check ANP and community prices and start a route.

Landing pages: `/como-usar/postos/`, `/como-usar/perfil-do-posto/`, `/como-usar/precos-do-posto/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`stations/01-stations-for-fuel.png`](stations/01-stations-for-fuel.png) | Tapping a fuel card opens the surveyed stations for that fuel, cheapest first. _(sao-paulo-sp)_ |
| 02 | [`stations/02-station-cards.png`](stations/02-station-cards.png) | Each station card shows the price, the brand, the public ANP address and the collection date. _(sao-paulo-sp)_ |
| 03 | [`stations/03-choose-fuel.png`](stations/03-choose-fuel.png) | Change the fuel from the selector at the top of the list. _(sao-paulo-sp)_ |
| 04 | [`stations/04-search-by-name.png`](stations/04-search-by-name.png) | Type part of a station name to filter the list. _(sao-paulo-sp)_ |
| 05 | [`stations/05-station-profile.png`](stations/05-station-profile.png) | The station page shows the name, brand, address and the main actions. _(sao-paulo-sp)_ |
| 06 | [`stations/06-community-and-anp-prices.png`](stations/06-community-and-anp-prices.png) | Choose a fuel chip; community prices (beta) and the ANP reference appear in separate cards. _(sao-paulo-sp)_ |
| 07 | [`stations/07-anp-reference-price.png`](stations/07-anp-reference-price.png) | The ANP reference price always shows the collection date and the survey week. _(sao-paulo-sp)_ |
| 08 | [`stations/08-fuel-quality.png`](stations/08-fuel-quality.png) | "Consultar análises de qualidade" lists ANP quality sampling for the station, with its limits. _(sao-paulo-sp)_ |
| 09 | [`stations/09-allow-location-for-route.png`](stations/09-allow-location-for-route.png) | "Rota e ir" asks for the device location the first time (Android system dialog). _(sao-paulo-sp; Android system screen)_ |
| 10 | [`stations/10-driver-experience.png`](stations/10-driver-experience.png) | Ratings and comments live in "Experiência dos motoristas"; this build shows the verification notice because station feedback is behind a feature flag. _(sorriso-mt; signed in)_ |
| 11 | [`stations/11-suggest-correction.png`](stations/11-suggest-correction.png) | "Sugerir correção" sends a name, address or permanent-closure correction for review. _(sorriso-mt; signed in)_ |
| 12 | [`stations/12-represent-your-station.png`](stations/12-represent-your-station.png) | "Entender como funciona" explains how a business can represent its station; it is not available in the beta. _(sorriso-mt; signed in)_ |

### Offline map download

Download the offline map of the region so the map and routes work without internet.

Landing pages: `/como-usar/offline-e-recuperacao/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`map-offline/01-download-suggestion.png`](map-offline/01-download-suggestion.png) | Opening the map of a city without a pack offers "Baixar mapa de Mato Grosso"; tap "Baixar". _(sorriso-mt)_ |
| 02 | [`map-offline/02-download-progress.png`](map-offline/02-download-progress.png) | Downloads shows the total progress and each package (region, world base map, coastlines); Wi-Fi only by default. _(sorriso-mt)_ |
| 03 | [`map-offline/03-offline-pack-active.png`](map-offline/03-offline-pack-active.png) | When it finishes, the region pack is listed as active. _(sorriso-mt)_ |
| 04 | [`map-offline/04-offline-map-ready.png`](map-offline/04-offline-map-ready.png) | Back on the map, streets and stations render from the offline pack. _(sorriso-mt)_ |

### Map, station sheet and navigation

Explore stations on the map, filter by source and service, open a station, save it and navigate with turn-by-turn guidance.

Landing pages: `/como-usar/postos/`, `/como-usar/offline-e-recuperacao/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`map/01-stations-on-map.png`](map/01-stations-on-map.png) | The map shows fuel stations (orange) and EV chargers (teal) of the chosen city. _(sorriso-mt)_ |
| 02 | [`map/02-map-filters.png`](map/02-map-filters.png) | "Filtros" chooses the city, the price source (ANP or community), the service (fuel or charging) and the product. _(sorriso-mt)_ |
| 03 | [`map/03-station-sheet.png`](map/03-station-sheet.png) | Tap a station to see its price with source, survey week and collection date. _(sorriso-mt)_ |
| 04 | [`map/04-save-favorite.png`](map/04-save-favorite.png) | "Salvar nos favoritos" keeps the station on this device. _(sorriso-mt)_ |
| 05 | [`map/05-route-preview.png`](map/05-route-preview.png) | "Rota e ir" draws the route from the current position with time and distance. _(sorriso-mt)_ |
| 06 | [`map/06-route-maneuvers.png`](map/06-route-maneuvers.png) | "Manobras" lists every turn before you leave. _(sorriso-mt)_ |
| 07 | [`map/07-turn-by-turn-navigation.png`](map/07-turn-by-turn-navigation.png) | "Iniciar navegação" starts offline turn-by-turn guidance with voice, zoom and recentre controls. _(sorriso-mt)_ |
| 08 | [`map/08-ev-charger-sheet.png`](map/08-ev-charger-sheet.png) | EV chargers come from open sources; the sheet says ANP has no electricity tariffs. _(sorriso-mt)_ |

### ANP survey weeks offline

Download earlier ANP survey weeks to read them offline and switch the active week.

Landing pages: `/como-usar/semanas-anp/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`survey-weeks/01-open-week-picker.png`](survey-weeks/01-open-week-picker.png) | Tap the week chip at the top to open "Semana de pesquisa". _(sorriso-mt)_ |
| 02 | [`survey-weeks/02-week-list.png`](survey-weeks/02-week-list.png) | Each week shows when it was updated and whether it is downloaded; "Baixar" saves it offline. _(sorriso-mt)_ |
| 03 | [`survey-weeks/03-downloading-week.png`](survey-weeks/03-downloading-week.png) | The download imports the ANP data in place and can be cancelled. _(sorriso-mt)_ |
| 04 | [`survey-weeks/04-week-downloaded.png`](survey-weeks/04-week-downloaded.png) | Downloaded weeks are marked "Baixada". _(sorriso-mt)_ |
| 05 | [`survey-weeks/05-older-week-active.png`](survey-weeks/05-older-week-active.png) | Tapping a downloaded week makes it active: the chip and the ANP reference line change. _(sorriso-mt)_ |

### Price history

Compare the ANP average of a fuel across the downloaded weeks.

Landing pages: `/como-usar/historico/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`history/01-weekly-history.png`](history/01-weekly-history.png) | "Histórico" charts the city average for each downloaded week, newest first. _(sorriso-mt)_ |
| 02 | [`history/02-choose-fuel.png`](history/02-choose-fuel.png) | Change the fuel from the selector. _(sorriso-mt)_ |
| 03 | [`history/03-diesel-s10-trend.png`](history/03-diesel-s10-trend.png) | Diesel S10 history for the same weeks. _(sorriso-mt)_ |
| 04 | [`history/04-tap-a-week.png`](history/04-tap-a-week.png) | Tap a bar to read that week's value. _(sorriso-mt)_ |

### Vehicles, tank-fill cost and fuel log

Register up to three vehicles, see the cost to fill the tank, turn on price-drop alerts and log fill-ups.

Landing pages: `/como-usar/veiculos/`, `/ferramentas/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`vehicles/01-no-vehicles.png`](vehicles/01-no-vehicles.png) | "Veículos" starts empty; the + button adds a vehicle. _(sorriso-mt)_ |
| 02 | [`vehicles/02-name-and-tank.png`](vehicles/02-name-and-tank.png) | Give the vehicle a name, choose the propulsion and type the tank capacity in litres. _(sorriso-mt)_ |
| 03 | [`vehicles/03-fuel-and-price-alert.png`](vehicles/03-fuel-and-price-alert.png) | Choose the fuel and the price reference, and switch on "Avisar quando o preço cair" for a local price-drop alert. _(sorriso-mt)_ |
| 04 | [`vehicles/04-vehicle-saved.png`](vehicles/04-vehicle-saved.png) | The saved vehicle lists its fuel, tank size and price reference. _(sorriso-mt)_ |
| 05 | [`vehicles/05-tank-fill-cost.png`](vehicles/05-tank-fill-cost.png) | The home screen shows the cost to fill the tank at the cheapest surveyed station and a shortcut to go there. _(sorriso-mt)_ |
| 06 | [`vehicles/06-fuel-log.png`](vehicles/06-fuel-log.png) | "Lançamentos" keeps the vehicle's fill-ups and expenses on the device. _(sorriso-mt)_ |
| 07 | [`vehicles/07-add-fill-up.png`](vehicles/07-add-fill-up.png) | Record litres, full or partial tank, total paid and odometer. _(sorriso-mt)_ |
| 08 | [`vehicles/08-fuel-log-entry.png`](vehicles/08-fuel-log-entry.png) | The entry appears in the log, stored locally. _(sorriso-mt)_ |

### Settings

Sync, automatic downloads, language, search distance, data retention, storage and account links.

Landing pages: `/como-usar/configuracoes/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`settings/01-sync-and-preferences.png`](settings/01-sync-and-preferences.png) | Sync now, change the survey week and choose background sync and Wi-Fi-only rules. _(sorriso-mt)_ |
| 02 | [`settings/02-language-and-search-distance.png`](settings/02-language-and-search-distance.png) | Pick the app language and the radius of "Mais barato perto de mim". _(sorriso-mt)_ |
| 03 | [`settings/03-data-retention-and-city-directory.png`](settings/03-data-retention-and-city-directory.png) | Choose how many weeks of station data to keep and whether to keep the city's public directory offline. _(sorriso-mt)_ |
| 04 | [`settings/04-storage-account-and-about.png`](settings/04-storage-account-and-about.png) | See storage use, clear caches, open account and privacy, share the app and the official ANP page. _(sorriso-mt)_ |

### Profile and tools

The Profile tab groups account, tools, references, Premium status and project contributors.

Landing pages: `/como-usar/conta-e-privacidade/`, `/contribuidores/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`profile-and-tools/01-tools-list.png`](profile-and-tools/01-tools-list.png) | "Ferramentas e referências": games, company mode, vehicles, settings and contributions. _(sorriso-mt)_ |
| 02 | [`profile-and-tools/02-more-tools.png`](profile-and-tools/02-more-tools.png) | More tools: ANP history, surveyed stations, survey week, help and privacy. _(sorriso-mt)_ |
| 03 | [`profile-and-tools/03-premium-status.png`](profile-and-tools/03-premium-status.png) | Premium shows the plan status; paid purchase is disabled in this version and never affects trust. _(sorriso-mt; signed in)_ |
| 04 | [`profile-and-tools/04-project-contributors.png`](profile-and-tools/04-project-contributors.png) | "Contribuidores do projeto" lists approved contributor profiles. _(sorriso-mt; signed in)_ |

> Not captured (after step 04): "Modo empresa", "Entrar em uma empresa" and "Meus relatos privados" are FLAG_SECURE screens and are not captured.

### Help, rules and privacy

Community rules, reporting and appeals, privacy summary and the terms accepted on this device.

Landing pages: `/como-usar/ajuda-e-regras/`, `/como-usar/conta-e-privacidade/`, `/privacidade/`, `/termos/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`help-and-privacy/01-help-and-community-rules.png`](help-and-privacy/01-help-and-community-rules.png) | "Ajuda e regras da comunidade": how to contribute well, community rules, reports and appeals. _(sorriso-mt)_ |
| 02 | [`help-and-privacy/02-privacy-and-terms.png`](help-and-privacy/02-privacy-and-terms.png) | "Privacidade": full policy, web account deletion and the terms version accepted on this device. _(sorriso-mt)_ |

### Free account: create and delete

Create a free account with only a username, see the signed-in profile and delete the account.

Landing pages: `/como-usar/conta-e-privacidade/`, `/excluir-conta/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`account/01-guest-profile.png`](account/01-guest-profile.png) | As a guest, Profile offers "Entrar ou criar conta gratuita"; browsing never needs an account. _(sorriso-mt)_ |
| 02 | [`account/02-signed-in-profile.png`](account/02-signed-in-profile.png) | After creating the account (username only, no email or password) Profile shows "Conectado". _(sorriso-mt; signed in)_ |
| 03 | [`account/03-after-account-deletion.png`](account/03-after-account-deletion.png) | After "Gerenciar conta → Excluir minha conta → Sim, excluir" the app returns to guest mode. _(sorriso-mt)_ |

> Not captured (after step 01): Type a username (3–20 letters and numbers, starting with a letter) and tap "Criar conta". The account screens are protected with FLAG_SECURE so the account key never appears in screenshots.

> Not captured (after step 02): To delete: Profile → "Gerenciar conta" → "Excluir minha conta" → "Sim, excluir". This screen is FLAG_SECURE; the web path is /excluir-conta/.

### Driver hub

Quick fuel and charging expenses, shifts and earnings for app drivers.

Landing pages: `/como-usar/veiculos/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`driver-hub/01-driver-hub.png`](driver-hub/01-driver-hub.png) | "Central do motorista" shows the vehicle in use, quick expense entries and work tracking (Premium gate shown). _(sorriso-mt; signed in)_ |
| 02 | [`driver-hub/02-shift-and-earnings.png`](driver-hub/02-shift-and-earnings.png) | "Turno e ganhos": start a shift with the odometer and record the day's net earnings. _(sorriso-mt; signed in)_ **Known issue:** Shift and earnings labels are hard-coded Portuguese in DriverWorkScreen.kt (not string resources). |

### Community tab

Entry point for contributing prices, the forum and the community price feed.

Landing pages: `/como-usar/comunidade/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`community/01-community-tab.png`](community/01-community-tab.png) | "Comunidade" opens the price contribution, the Abastevo forum and the community price feed. _(sorriso-mt; signed in)_ |
| 02 | [`community/02-community-prices-empty.png`](community/02-community-prices-empty.png) | The feed filters by order and fuel; with no recent community price it invites a contribution. _(sorriso-mt; signed in)_ |

### Forum: read, comment, post and block

Read editorial posts, filter by category, react and comment, write a topic and manage blocked users.

Landing pages: `/como-usar/comunidade/`, `/como-usar/denunciar-abuso/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`forum/01-forum-feed.png`](forum/01-forum-feed.png) | "Fórum Abastevo" lists posts with author, category and time; the pencil starts a new topic. _(sorriso-mt; signed in)_ |
| 02 | [`forum/02-filter-by-category.png`](forum/02-filter-by-category.png) | Category tabs filter the feed (fuels, e-mobility, traffic law…). _(sorriso-mt; signed in)_ |
| 03 | [`forum/03-read-article.png`](forum/03-read-article.png) | Open a post to read it in full. _(sorriso-mt; signed in)_ |
| 04 | [`forum/04-reactions-and-comments.png`](forum/04-reactions-and-comments.png) | At the end: comment, like, dislike, share and the live comment list. _(sorriso-mt; signed in)_ |
| 05 | [`forum/05-write-comment.png`](forum/05-write-comment.png) | Write a comment of up to 280 characters and tap "Publicar" (new comments go through moderation). _(sorriso-mt; draft, not submitted; signed in)_ |
| 06 | [`forum/06-new-topic-draft.png`](forum/06-new-topic-draft.png) | "Novo tópico": title, category and content. _(sorriso-mt; draft, not submitted; signed in)_ **Known issue:** Category chips show internal codes (FUELS, ELECTRIC, TRAFFIC_RULES) instead of translated labels. |
| 07 | [`forum/07-new-topic-options.png`](forum/07-new-topic-options.png) | Optional state, IBGE municipality, fuel and public name, then "Publicar tópico". _(sorriso-mt; draft, not submitted; signed in)_ **Known issue:** Fuel chips show internal codes (GASOLINE_REGULAR, GASOLINE_PREMIUM). |
| 08 | [`forum/08-blocked-users.png`](forum/08-blocked-users.png) | "Gerenciar conta → Usuários bloqueados" lists blocked people; they are not notified and can be unblocked any time. _(sorriso-mt; signed in)_ |

> Not captured (after step 07): On a topic or comment written by someone else, the ⋮ menu offers "Denunciar tópico", "Denunciar comentário", "Denunciar usuário" and "Bloquear usuário". Staging had only official editorial posts, which do not show the menu, so these dialogs were not captured.

### Contribute a price with a photo

Choose the station, photograph the price board, review the reading or type the price, and send it to the community queue.

Landing pages: `/como-usar/fotografar-preco/`, `/como-usar/minhas-contribuicoes/`

| Step | Image | Caption |
|---|---|---|
| 01 | [`contribute-price/01-choose-station.png`](contribute-price/01-choose-station.png) | "Colaborar com preço" first asks which station the price belongs to. _(sorriso-mt; signed in)_ |
| 02 | [`contribute-price/02-search-station.png`](contribute-price/02-search-station.png) | Search the station by name. _(sorriso-mt; signed in)_ |
| 03 | [`contribute-price/03-station-selected.png`](contribute-price/03-station-selected.png) | With the station selected, tap "Fotografar preço". _(sorriso-mt; signed in)_ |
| 04 | [`contribute-price/04-allow-camera.png`](contribute-price/04-allow-camera.png) | Android asks for camera permission the first time. _(sorriso-mt; Android system screen; signed in)_ |
| 05 | [`contribute-price/05-phone-camera.png`](contribute-price/05-phone-camera.png) | The phone camera opens; photograph only the price board. (Emulator test scene — not a price board.) _(sorriso-mt; Android system screen; emulator test scene; signed in)_ |
| 06 | [`contribute-price/06-photo-attached.png`](contribute-price/06-photo-attached.png) | The photo is attached; crop to the frame or read it again. With no readable board the app asks you to type the prices. _(sorriso-mt; emulator test scene; signed in)_ |
| 07 | [`contribute-price/07-type-price-and-send.png`](contribute-price/07-type-price-and-send.png) | Add the fuel, type the price (R$ 6,25) and tap "Enviar"; blank fuels are not sent. _(sorriso-mt; draft, not submitted; signed in)_ **Known issue:** Send button reads "Enviar 1 preços" (plural not handled). |
| 08 | [`contribute-price/08-my-contributions.png`](contribute-price/08-my-contributions.png) | "Minhas contribuições" tracks sends, queue and cancellations on this device only. _(sorriso-mt; signed in)_ |

### Offline games

Two offline mini-games with local records only.

| Step | Image | Caption |
|---|---|---|
| 01 | [`games/01-games-hub.png`](games/01-games-hub.png) | "Games": play offline, even without an account; records stay on the device. _(sorriso-mt)_ |
| 02 | [`games/02-second-game.png`](games/02-second-game.png) | Fuel Run and accessibility options: mute, reduce motion, save battery. _(sorriso-mt)_ |
| 03 | [`games/03-how-to-play.png`](games/03-how-to-play.png) | Each game explains how to play before "Começar". _(sorriso-mt)_ |
| 04 | [`games/04-match-over.png`](games/04-match-over.png) | End of a match with the score and the local record. _(sorriso-mt)_ |

<!-- END GENERATED STEPS -->
