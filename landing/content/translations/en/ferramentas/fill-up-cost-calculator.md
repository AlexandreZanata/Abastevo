---
title: "How much it costs to fill the tank: calculator"
headline: "Calculator: how much it costs to fill the tank"
description: "Find out how many liters are missing and how much you will pay to fill the tank from the current gauge level, and estimate the car's range with a full tank."
summary: "Liters and cost to fill the tank from the gauge level, with the estimated range."
slug: fill-up-cost-calculator
primaryQuery: "fill-up cost calculator"
datePublished: 2026-11-21
dateModified: 2026-11-21
translationOf: /ferramentas/encher-o-tanque/
sourceRevision: 2026-11-17
translationStatus: reviewed
reviewedBy: AI review (Codex)
reviewedAt: 2026-11-22
related:
  - /en/guides/how-to-read-fuel-prices/
  - /en/guides/anp-weekly-price-survey/
  - /en/guides/ethanol-or-gasoline/
---

## How the math works

The fuel gauge shows a fraction of the tank. The missing part, multiplied by
the tank capacity, gives the liters needed to fill up.

```
liters to fill up = capacity × (1 − current fraction)
cost = liters to fill up × price per liter
range with a full tank = capacity × consumption (km/l)
```

The tank capacity is in the vehicle manual. If you do not know your
consumption, use the
[fuel consumption calculator](/en/tools/fuel-consumption-calculator/).

## Worked example

A 48-liter tank, the gauge at 1/4 and fuel at R$ 6.29 per liter, in a car that
does 12 km/l:

1. 3/4 of the tank is left to fill: 48 × 0.75 = 36 liters.
2. Cost to fill up: 36 × 6.29 = R$ 226.44.
3. The whole tank, empty to full, would cost 48 × 6.29 = R$ 301.92.
4. Estimated range with a full tank: 48 × 12 = 576 km.

The values are illustrative.

## The Brazilian context

Prices are per liter in Brazilian reais (BRL), and the price on the sign
already includes federal and state taxes. At most stations an attendant fills
the tank for you; you do not operate the pump. Pix, Brazil's instant payment
system, and cards are common payment methods.

## Limits of the estimate

- The gauge is not precise: in many cars it is slow to drop at first and falls
  faster toward the end. Treat the result as an approximation.
- The real capacity can be a little different from the one stated in the
  manual, and being on reserve does not mean the tank is dry.
- At the pump, check that the display starts at zero and that the price is the
  same as on the sign. Your rights are in
  [consumer rights at the station](/guias/direitos-do-consumidor-no-posto/)
  (in Portuguese).

To know whether the price per liter is high or low in your region, compare it
with the [reference prices by state](/en/fuel-prices/), always looking
at the collection date.

## In the app

The abastevo Android app, still in development, does similar math for the
whole tank, with the week's reference price:

1. In "Vehicles", tap the "+" button and enter the "Vehicle name" and the
   "Tank capacity (liters)".
2. Choose the "Fuel type" and the "Price reference": "Cheapest station" or
   "Specific station".
3. On the home screen, the "Tank fill cost" card shows the estimate for the
   whole tank.

The card considers the whole tank, empty to full; to fill up from the gauge
level, use the calculator on this page. The captures are on the Portuguese
page, show the Portuguese interface and were taken on November 17, 2026. Their
example is an estimate for a vehicle with a 50-liter tank in Sorriso, Mato
Grosso, with the reference from ANP, Brazil's oil and fuels regulator, for
November 8 to 14, 2026. See the tutorial
[Vehicles and tank estimate](/como-usar/veiculos/) (in Portuguese).
