# Play Store Listing Draft

> **Status:** Draft for v3.1.0  
> **Distribution:** Sideload via [GitHub Releases](https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS/releases) is the primary path until Play Console submission.  
> **Product contract:** [user-business-logic.md](user-business-logic.md) · **Privacy:** [privacy-policy.md](privacy-policy.md)

---

## Store metadata

| Field | Value |
|-------|-------|
| **Package name** | `com.anpfuel.app` |
| **Category** | Auto & Vehicles |
| **Tags** | fuel prices, ANP, gasoline, ethanol, diesel, Brazil, offline |
| **Content rating** | Everyone (no sensitive content; public government data) |
| **Pricing** | Free |
| **Ads** | No |
| **In-app purchases** | No |
| **Min Android** | API 26 (Android 8.0) |
| **Target Android** | API 35 |
| **Privacy policy URL** | `https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS/blob/main/docs/privacy-policy.md` |

### Data safety (Google Play form)

| Question | Answer |
|----------|--------|
| Collects or shares user data? | **No** — vehicle names are user-entered labels stored on device only |
| Optional location used? | **Yes, one-shot** — only if user taps Use my location; coordinates not stored |
| Local notifications? | **Optional** — price drop alerts generated on device after sync |
| Data encrypted in transit? | **Yes** (HTTPS to gov.br and Nominatim when used) |
| Users can request data deletion? | **N/A** — no account; clear cache in Settings (UC-008) |
| Independent security review? | **No** |

---

## English listing (default)

### App name (max 30 characters)

```
ANP Fuel Prices
```

### Short description (max 80 characters)

<!-- begin:short-description-en -->
Official ANP weekly fuel prices by city. Offline, free, open source.
<!-- end:short-description-en -->

### Full description (max 4000 characters)

<!-- begin:full-description-en -->
Browse official Brazilian fuel prices from ANP (National Agency of Petroleum, Natural Gas and Biofuels) — directly on your phone.

**Features**
• Average gasoline, ethanol, and diesel prices by municipality
• **Tank fill cost** — register vehicles and see estimated full-tank price for your city
• **Price drop alerts** — optional local notifications when your tracked fuel price falls week over week
• Search any Brazilian city with autocomplete
• Works offline after the first sync
• Optional per-station prices (CNPJ, brand, address) with **Navigate** to Maps/Waze
• Optional **Use my location** during onboarding to detect your city (coordinates not stored)
• Price history trends for your selected city
• English and Brazilian Portuguese UI
• No account, no ads, no backend — 100% on-device

**How it works**
ANP publishes weekly price surveys on gov.br. The app downloads public spreadsheets, imports them locally, and lets you explore averages and station detail without a server.

**Important**
• Prices are weekly surveys, not real-time pump prices
• Data freshness depends on ANP publication schedule
• This app is not affiliated with ANP
• Source: ANP — Agência Nacional do Petróleo, Gás Natural e Biocombustíveis

**Open source**
MIT licensed. Source code: https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS

**Privacy**
No personal data collected. See privacy policy in the repository.
<!-- end:full-description-en -->

---

## Portuguese listing (pt-BR)

### App name

```
ANP Fuel Prices
```

### Short description (max 80 characters)

<!-- begin:short-description-pt -->
Preços semanais oficiais da ANP por cidade. Offline, grátis e open source.
<!-- end:short-description-pt -->

### Full description (max 4000 characters)

<!-- begin:full-description-pt -->
Consulte preços oficiais de combustíveis da ANP (Agência Nacional do Petróleo, Gás Natural e Biocombustíveis) — direto no seu celular.

**Recursos**
• Médias de gasolina, etanol e diesel por município
• **Custo do tanque** — cadastre veículos e veja o preço estimado de encher o tanque na sua cidade
• **Alertas de queda de preço** — notificações locais opcionais quando o combustível acompanhado cair entre semanas
• Busca de cidades brasileiras com autocompletar
• Funciona offline após a primeira sincronização
• Preços por posto (CNPJ, bandeira, endereço) com botão **Ir** para Maps/Waze
• **Usar minha localização** opcional no onboarding para detectar sua cidade (coordenadas não armazenadas)
• Histórico de preços da cidade selecionada
• Interface em português (Brasil) e inglês
• Sem conta, sem anúncios, sem servidor — 100% no aparelho

**Como funciona**
A ANP publica pesquisas semanais em gov.br. O app baixa planilhas públicas, importa localmente e permite explorar médias e detalhes de revendas sem backend.

**Importante**
• Preços são pesquisas semanais, não valores em tempo real no posto
• A atualização depende do calendário de publicação da ANP
• Este app não é afiliado à ANP
• Fonte: ANP — Agência Nacional do Petróleo, Gás Natural e Biocombustíveis

**Código aberto**
Licença MIT. Código-fonte: https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS

**Privacidade**
Nenhum dado pessoal coletado. Política de privacidade no repositório.
<!-- end:full-description-pt -->

---

## Required disclaimers (must appear in store copy)

- [x] ANP data attribution (BR-009)
- [x] Not affiliated with ANP
- [x] Weekly data — not real-time prices
- [x] Offline-capable after first sync
- [x] No personal data collected (links to privacy policy)
- [x] Open-source MIT license and repository URL

---

## Graphics checklist (not yet produced)

| Asset | Spec | Status |
|-------|------|--------|
| App icon | 512 × 512 PNG | Use `@mipmap/ic_launcher` export |
| Feature graphic | 1024 × 500 PNG | Pending |
| Phone screenshots | ≥ 2, 16:9 or 9:16 | Pending — Home, Search, Prices, Settings |
| 7-inch tablet screenshots | Optional | Pending |
| Promo video | Optional | N/A |

Suggested screenshot captions (pt-BR primary market):

1. Home — médias da semana na sua cidade
2. Home — custo do tanque com veículo cadastrado
3. Veículos — cadastro e alerta de queda de preço
4. Postos — botão Ir para Maps/Waze
5. Busca — encontre qualquer município
6. Offline — dados em cache sem internet

---

## Sideload distribution (v1 primary)

Until Play Store submission:

1. Publish signed APK/AAB on GitHub Releases (Phase 10.5–10.6).
2. Users enable “Install unknown apps” for their browser/files app.
3. README and release notes link to ANP attribution and offline disclaimer.

Release notes must repeat:

> Fuel price data © ANP (public government data). This app is not affiliated with ANP. Prices reflect weekly surveys, not live pump prices.

---

## Pre-submission checklist

- [ ] Signed release build (10.5)
- [ ] Screenshots captured on emulator + physical device
- [ ] Privacy policy URL live on `main` branch
- [ ] Data safety form completed in Play Console
- [ ] ANP attribution visible in-app (BR-009) — verified in MVP script
- [ ] Store listing reviewed in both `en` and `pt-BR`
