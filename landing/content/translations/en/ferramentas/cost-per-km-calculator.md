---
title: "Cost per km: combustion or electric car"
headline: "Cost per km calculator: combustion or electric"
description: "Compare the energy cost per kilometer of a gasoline, ethanol or diesel car with that of an electric car, using the price per liter and the rate per kWh."
summary: "Energy cost per kilometer of a combustion car compared with an electric car."
slug: cost-per-km-calculator
primaryQuery: "cost per km calculator"
datePublished: 2026-11-21
dateModified: 2026-11-21
translationOf: /ferramentas/custo-por-km/
sourceRevision: 2026-11-17
translationStatus: reviewed
reviewedBy: AI review (Claude)
reviewedAt: 2026-11-21
related:
  - /en/guides/measure-fuel-economy/
  - /en/guides/is-cng-worth-it/
  - /en/guides/how-gasoline-prices-are-formed/
  - /en/fuel-prices/
---

## How the math works

To compare cars with different energy sources, the fairest measure is the
energy cost per kilometer driven.

```
combustion: cost per km = price per liter ÷ consumption (km/l)
electric: cost per km = electricity rate (R$/kWh) ÷ consumption (km/kWh)
monthly cost = cost per km × kilometers per month
```

If the electric car's consumption is given in kWh per 100 km, convert it like
this: km/kWh = 100 ÷ (kWh/100 km). For example, 15 kWh/100 km equals
6.7 km/kWh.

The electricity rate must be the final price per kWh, with taxes: the one on
your electricity bill, if you charge at home, or the one charged by the
charging station. To find the official consumption of each model, check the
label of the Brazilian Vehicle Labeling Program (Programa Brasileiro de
Etiquetagem Veicular) on the website of
[Inmetro](https://www.gov.br/inmetro/), Brazil's national metrology institute.

## Worked example

A gasoline car that does 12 km/l, with fuel at R$ 6.29 per liter, compared
with an electric car that does 6.5 km/kWh, charged at R$ 0.95 per kWh, driving
1,200 km per month:

1. Combustion: 6.29 ÷ 12 = R$ 0.524 per km.
2. Electric: 0.95 ÷ 6.5 = R$ 0.146 per km.
3. Difference: R$ 0.378 per km in favor of the electric car.
4. Per month: R$ 629.00 on gasoline and R$ 175.38 with the electric car.

The values are illustrative; they do not reflect any specific rate or price.

## The Brazilian context

Fuel prices are per liter in Brazilian reais (BRL), and the price on the sign
already includes federal and state taxes. Brazilian gasoline is a blend that
contains anhydrous ethanol.

## What the math does not include

- Purchase price, insurance, IPVA (the annual state vehicle tax), maintenance
  and depreciation, which often weigh more than energy.
- Charging losses: part of the energy that leaves the outlet does not reach
  the battery, so the real cost per km of the electric car is a little higher
  than the calculated one.
- Tariff flags (bandeiras tarifárias, the surcharge levels on Brazilian
  electricity bills) and different rates by time of day, which change the
  price per kWh over the year.

To understand why a liter costs what it costs, read
[how gasoline prices are formed](/en/guides/how-gasoline-prices-are-formed/).

## In the app

The abastevo Android app, still in development, provides two numbers that are
useful for this math:

1. With a vehicle registered in "Vehicles", the "Tank fill cost" card on the
   home screen shows the week's reference price applied to your tank.
2. On a station's details screen, "Route and go" shows the distance of the
   route. Multiply the distance by the cost per kilometer to estimate the cost
   of a detour.

The captures are on the Portuguese page, show the Portuguese interface and
were taken on November 17, 2026. Their example is an estimate for a 50 L tank
in Sorriso, Mato Grosso, with the reference from ANP, Brazil's oil and fuels
regulator, for November 8 to 14, 2026, and a route to a station summarized as
8 min · 5.1 km. See the tutorials
[Vehicles and tank estimate](/en/how-to/vehicles-and-tank-estimate/) and
[Find and navigate to a station](/en/how-to/find-and-navigate-to-a-station/).
