---
title: "How Brazil's ANP weekly fuel price survey works"
description: "Understand the methodology of ANP's weekly fuel price survey in Brazil, its sampling, why it is not real time and how the abastevo community complements the data."
summary: "Weekly sampling, what the official reference measures and how it lives alongside community observations."
slug: anp-weekly-price-survey
category: dados
tags: [anp, price-survey, open-data, source-and-date]
primaryQuery: "how the anp fuel price survey works"
datePublished: 2026-11-21
dateModified: 2026-11-21
translationOf: /guias/pesquisa-anp/
sourceRevision: 2026-11-17
translationStatus: reviewed
reviewedBy: AI review (Claude)
reviewedAt: 2026-11-21
related:
  - /en/guides/how-to-read-fuel-prices/
  - /en/guides/how-gasoline-prices-are-formed/
  - /en/guides/ethanol-or-gasoline/
  - /en/tools/ethanol-or-gasoline-calculator/
---

ANP, Brazil's oil and fuels regulator (Agência Nacional do Petróleo, Gás
Natural e Biocombustíveis), periodically surveys fuel prices and marketing
margins in Brazil. Understand how this survey works, what its reach is and why
it serves as a regulatory and historical reference, but not as real-time
information.

## What the ANP price survey is

The ANP survey is an official statistical survey meant to monitor the prices
charged by the retail market (retail stations) and by distributors in several
regions of the country. It covers common automotive fuels, such as regular
gasoline and additive gasoline (gasolina aditivada), hydrous ethanol (E100),
diesel and CNG (compressed natural gas for vehicles, called GNV in Brazil).

The survey is carried out by research companies hired by the agency through
public bidding, following predefined statistical criteria.

## Sampling methodology and coverage

The ANP survey does not visit every fuel station in Brazil. The country has
more than 40,000 authorized retail stations, which makes an exhaustive
in-person check of each one every few days unfeasible.

For that reason, the agency adopts **stratified probability sampling**:

- Only a share of Brazilian municipalities is surveyed regularly (generally
  state capitals, metropolitan regions and municipalities of greater economic
  or population relevance).
- Within each selected municipality, a sample of stations is drawn to receive
  the field researchers.
- Smaller municipalities or stations on remote highways are often not part of
  the periodic sample.

## Frequency and the time lag

ANP field data are normally collected over the business days of a week. After
collection, the data go through stages of consolidation, statistical checking
and validation before being published in the official reports and on the
government's open data portal.

::: atencao
**Mind the lag:** when an ANP report is published (generally at the end of the
week or at the beginning of the following week), the prices recorded refer to
the earlier week in which they were collected. If a station changed its price
on Tuesday afternoon, the official ANP data will not reflect that change
immediately.
:::

## ANP reference vs. community collaboration

In the **abastevo** project, we treat the ANP survey with total clarity about
its nature:

- **Regulatory reference:** ANP data are shown with the exact date of the
  reference week they belong to, so the driver has a reliable official
  benchmark of the average in the region.
- **Real-time reality:** to know the price that is at the pump today, the
  project relies on collaborative observations sent by the community of
  drivers in the app, with the exact date and time of each record.

## Access to open data and independence

ANP's consolidated data are public and accessible to any citizen through the
official portal of the federal government at
[gov.br/anp](https://www.gov.br/anp) and on the Brazilian Open Data Portal.

abastevo is an independent initiative whose code is under the Abastevo Visible
Attribution License. **We have no government affiliation or institutional link
with ANP**, and use only the public data made available by the agency, in
compliance with open data legislation.

## In the app

The abastevo Android app is in development. The captures that illustrate this
section are on the Portuguese page: they are real, were taken on November 17,
2026, and show the ANP reference of the week indicated, not the pump price. To
see the weekly survey inside the app:

1. On the home screen, the "ANP reference" line, below "Prices in your city",
   gives the survey period. Each card carries the "ANP" badge and the number
   of stations surveyed.
2. Tap the week next to the city name to open "Survey week". The "Each week
   includes" summary lists what comes in each week: weekly average prices and
   prices by retail station.
3. Tap "Download" to keep an earlier week on the device and compare it with
   the current one.

The captures show the app in Portuguese: the ANP reference in São Paulo, SP,
for the week of November 8 to 14, 2026; the "Survey week" panel on the home
screen in Sorriso, MT; and the weeks available to download, with the week of
November 8 to 14, 2026 marked as current and downloaded.

The full walkthrough is in the tutorials
[Check the ANP table](/como-usar/precos-anp/) (in Portuguese) and
[Switch or download a week](/como-usar/semanas-anp/) (in Portuguese).
