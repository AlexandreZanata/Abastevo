# Play Store listing — Abastevo

> **Status:** Draft for the first Play submission, rewritten 2026-11-15 for the
> current app (accounts, community, forum, offline maps). The earlier draft
> described a no-account, no-backend app and must not be used.
> **Readiness:** [docs/store/PLAY_STORE_READINESS.md](store/PLAY_STORE_READINESS.md)
> · **Graphics:** [docs/store/](store/README.md)
> · **Privacy:** [abastevo.com.br/privacidade](https://abastevo.com.br/privacidade/)

The Google Play metadata policy forbids ranking/price/promotion claims ("melhor",
"nº 1", "grátis", "novo"), calls to action and keyword lists in the title, short
description and graphics. Apps showing government data must cite the source and
state that they do not represent a government body. Keep both when editing.

## Store metadata

| Field | Value |
|-------|-------|
| **App name** | Abastevo |
| **Package name** | `com.anpfuel.app` (permanent after the first upload) |
| **Default language** | Português (Brasil) — `pt-BR`; translation: English (United States) |
| **Category** | Auto & Vehicles (to confirm) |
| **Website** | https://abastevo.com.br |
| **Contact email** | To be supplied by the owner (required by Play; never invent one) |
| **Privacy policy URL** | https://abastevo.com.br/privacidade/ |
| **Account deletion URL** | https://abastevo.com.br/excluir-conta/ |
| **Ads** | No |
| **In-app purchases** | No (no sale is enabled in the app) |
| **Government app** | No — independent app that uses public ANP data |
| **Min / target Android** | API 26 (Android 8.0) / API 36 (Android 16) |

Data safety, content rating and other App content answers:
[readiness §2 and §5](store/PLAY_STORE_READINESS.md#2-app-content-declarations-play-console--policy--app-content).

---

## Português (Brasil) — padrão

### Nome do app (máx. 30)

```
Abastevo
```

### Descrição curta (máx. 80)

<!-- begin:short-description-pt -->
Preços de combustíveis da ANP por cidade, mapa de postos e comunidade.
<!-- end:short-description-pt -->

### Descrição completa (máx. 4000)

<!-- begin:full-description-pt -->
O Abastevo reúne os preços de combustíveis da sua cidade em um só lugar: médias semanais da pesquisa da ANP, preços por posto, mapa e as informações compartilhadas pela comunidade de motoristas.

O QUE VOCÊ ENCONTRA
• Médias semanais de gasolina, etanol, diesel, GNV e GLP por município, com a semana de referência da pesquisa
• Lista de postos com preço, bandeira e endereço, do menor para o maior preço
• Mapa de postos e pontos de recarga, com rota até o posto; baixe o mapa da sua região para usar sem internet
• Custo para encher o tanque: cadastre até 3 veículos e veja quanto custa um tanque cheio na sua cidade
• Aviso opcional no celular quando o preço do combustível que você acompanha cair
• Histórico para comparar os preços semana a semana
• Semanas da pesquisa salvas no aparelho para consultar offline
• Comunidade: compartilhe o preço que você viu no posto, com foto opcional, e ajude outros motoristas
• Fórum Abastevo com guias e discussões sobre combustíveis, eletromobilidade e trânsito

Para consultar preços não é preciso criar conta. Para participar da comunidade, a conta é criada só com um nome de usuário.

DE ONDE VÊM OS DADOS
Os preços médios e por posto vêm do Levantamento de Preços de Combustíveis publicado pela ANP — Agência Nacional do Petróleo, Gás Natural e Biocombustíveis: https://www.gov.br/anp/pt-br
A pesquisa é semanal e feita por amostragem: não inclui todos os postos e não é o preço em tempo real na bomba. Os preços enviados pela comunidade aparecem identificados separadamente dos dados da ANP.

IMPORTANTE
• O Abastevo é um aplicativo independente. Não é afiliado à ANP e não representa nenhum órgão do governo.
• Confira sempre o preço no posto antes de abastecer.

PRIVACIDADE E CONTA
• Política de Privacidade: https://abastevo.com.br/privacidade/
• Termos de Uso: https://abastevo.com.br/termos/
• Excluir conta e dados: no app (Perfil › Gerenciar conta) ou em https://abastevo.com.br/excluir-conta/
<!-- end:full-description-pt -->

---

## English (United States)

### App name (max 30)

```
Abastevo
```

### Short description (max 80)

<!-- begin:short-description-en -->
Brazilian fuel prices from ANP by city, a station map and a driver community.
<!-- end:short-description-en -->

### Full description (max 4000)

<!-- begin:full-description-en -->
Abastevo brings together fuel prices for your Brazilian city: weekly averages from the ANP survey, per-station prices, a station map and prices shared by the driver community.

WHAT YOU GET
• Weekly average prices for gasoline, ethanol, diesel, CNG and LPG by municipality, with the survey reference week
• Station list with price, brand and address, sorted from the lowest price
• Map of fuel stations and EV chargers with directions; download your region's map to use it offline
• Tank fill cost: add up to 3 vehicles and see what a full tank costs in your city
• Optional on-device alert when the price of the fuel you follow drops
• Price history to compare week by week
• Survey weeks saved on your phone for offline use
• Community: share the price you saw at the pump, with an optional photo, to help other drivers
• Abastevo Forum with guides and discussions about fuel, e-mobility and traffic

Browsing prices needs no account. Joining the community only needs a username.

WHERE THE DATA COMES FROM
Average and per-station prices come from the fuel price survey published by ANP — Brazil's National Agency of Petroleum, Natural Gas and Biofuels: https://www.gov.br/anp/pt-br
The survey is weekly and sample-based: it does not cover every station and is not a real-time pump price. Community prices are labeled separately from ANP data.

IMPORTANT
• Abastevo is an independent app. It is not affiliated with ANP and does not represent any government body.
• Always check the price at the station before refueling.

PRIVACY AND ACCOUNT
• Privacy policy: https://abastevo.com.br/privacidade/
• Terms of use: https://abastevo.com.br/termos/
• Delete your account and data: in the app (Profile › Manage account) or at https://abastevo.com.br/excluir-conta/
<!-- end:full-description-en -->

---

## Graphics

All files are in [docs/store/listing-assets](store/listing-assets/) and are
regenerated by [docs/store/cards/build_cards.py](store/cards/build_cards.py).

| Play Console field | File(s) |
|---|---|
| App icon (512 × 512) | `icon-512.png` |
| Feature graphic (1024 × 500) | `feature-graphic-1024x500.png` |
| Phone screenshots (8) | `phone/01…08-*.png` (1080 × 1920) |
| 7-inch tablet screenshots (6) | `tablet-7in/01…06-*.png` (1080 × 1920) |
| 10-inch tablet screenshots (6) | `tablet-10in/01…06-*.png` (1440 × 2560) |

Suggested alt text (≤ 140 characters each, pt-BR), in phone order:

1. Médias semanais da ANP para gasolina, etanol e diesel em Sorriso, MT.
2. Mapa offline de São Paulo com postos de combustível.
3. Lista de postos de São Paulo ordenada do menor preço da gasolina.
4. Custo estimado para encher o tanque de 50 litros em Sorriso, MT.
5. Comunidade: compartilhar preços vistos no posto com a cidade.
6. Fórum Abastevo com artigos sobre a pesquisa da ANP e etanol ou gasolina.
7. Histórico semanal do preço da gasolina comum em Sorriso, MT.
8. Semanas da pesquisa da ANP baixadas para consulta offline.

## Release notes template (pt-BR)

```
Primeira versão no Google Play. Preços de combustíveis da ANP por cidade, mapa de postos com uso offline, custo do tanque, histórico, comunidade e fórum.
```
