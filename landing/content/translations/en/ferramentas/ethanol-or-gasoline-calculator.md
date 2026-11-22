---
title: "Ethanol or gasoline calculator for Brazil"
headline: "Calculator: ethanol or gasoline?"
description: "Compare ethanol and gasoline prices in Brazil, apply the 70% rule or your own consumption, and see the break-even ethanol price per liter."
summary: "Compare pump prices with the 70% rule or with your flex-fuel car's own consumption."
slug: ethanol-or-gasoline-calculator
primaryQuery: "ethanol or gasoline calculator"
datePublished: 2026-11-17
dateModified: 2026-11-21
translationOf: /ferramentas/etanol-ou-gasolina/
sourceRevision: 2026-11-17
translationStatus: reviewed
reviewedBy: AI review (Codex)
reviewedAt: 2026-11-22
related:
  - /en/guides/ethanol-or-gasoline/
  - /en/guides/measure-fuel-economy/
  - /en/guides/regular-or-additive-gasoline/
  - /en/fuel-prices/
---

## How the math works

Ethanol travels fewer kilometers per liter than gasoline because each liter
carries less energy. Looking only at which liter is cheaper is not enough: you
have to compare the cost of driving the same distance.

Divide the ethanol price by the gasoline price. The ratio is:

```
ratio = ethanol price ÷ gasoline price
```

- Below **70%** (0.70), ethanol tends to cost less per kilometer.
- Above **70%**, gasoline tends to cost less.
- The break-even ethanol price is the gasoline price multiplied by 0.70.

The 70% figure is an average. If you measured your own car with both fuels, use
the ratio between the two consumption figures (ethanol km/l ÷ gasoline km/l),
which is more accurate for that car. See
[how to calculate average consumption](/guias/como-calcular-consumo-medio/)
(in Portuguese).

## Worked example

Gasoline at R$ 6.29 and ethanol at R$ 4.19 per liter:

1. Ratio: 4.19 ÷ 6.29 = 0.666, so ethanol costs 66.6% of the gasoline price.
2. Break-even price by the 70% rule: 6.29 × 0.70 = R$ 4.403.
3. Since 66.6% is below 70%, ethanol tends to cost less per kilometer.

With measured consumption, for example 12.5 km/l on gasoline and 8.7 km/l on
ethanol, the reference becomes 8.7 ÷ 12.5 = 69.6%. The cost per kilometer is
R$ 0.482 on ethanol (4.19 ÷ 8.7) and R$ 0.503 on gasoline (6.29 ÷ 12.5):
ethanol still wins, by a small margin.

The prices above are illustrative; they are not prices from any station or
week.

## The Brazilian context

Brazilian gasoline is a blend: gasoline C contains the mandatory share of
anhydrous ethanol set by the federal government and published by ANP. Prices
are per liter in Brazilian reais (BRL), and the price on the sign already
includes federal and state taxes. At most stations an attendant fills the tank
for you; you do not operate the pump. Pix and cards are common payment
methods.

## Limits of the estimate

- The real ratio changes from car to car and with use: short trips and cold
  starts usually make ethanol perform worse.
- Very small differences (less than one percentage point) are inside the error
  margin of any home measurement.
- The price changes between stations and weeks. To compare your city, check the
  [reference prices by state](/en/fuel-prices/) with the collection
  date.

To understand where the 70% comes from and when it changes, read the guide
[ethanol or gasoline: the 70% rule](/en/guides/ethanol-or-gasoline/).

## In the app

The abastevo Android app, still in development, shows the two reference
prices from the same week:

1. Under "Prices in your city", look at the "Ethanol" and "Regular gasoline"
   cards and use the two values in the calculator on this page.
2. Under "Filters", "Lowest price" shows the lowest surveyed value of each
   fuel. Always compare values of the same kind: average with average, lowest
   with lowest.
3. In the "Abastevo Forum", the editorial post about ethanol or gasoline
   explains the same math.

The captures are on the Portuguese page, show the Portuguese interface and
were taken on November 17, 2026. The first one shows the ANP averages in São
Paulo, SP, for the week of November 8 to 14, 2026: the ethanol average divided
by the regular gasoline average is 4.09 ÷ 6.34 ≈ 0.65. See the tutorial
[Check the ANP table](/en/how-to/check-anp-prices/).
