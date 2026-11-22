---
title: "Cómo funciona la encuesta semanal de precios de la ANP"
description: "Conoce la metodología de la encuesta semanal de combustibles de la ANP, su muestreo, por qué no es en tiempo real y cómo la comunidad abastevo complementa los datos."
summary: "Muestreo semanal, qué mide la referencia oficial y cómo convive con las observaciones de la comunidad."
slug: encuesta-semanal-anp
category: dados
tags: [anp, encuesta-de-precios, datos-abiertos, fuente-y-fecha]
primaryQuery: "cómo funciona la encuesta de precios de la anp"
datePublished: 2026-11-21
dateModified: 2026-11-22
translationOf: /guias/pesquisa-anp/
sourceRevision: 2026-11-22
translationStatus: reviewed
reviewedBy: AI review (Codex)
reviewedAt: 2026-11-22
related:
  - /es/guias/como-leer-los-precios/
  - /es/guias/como-se-forma-el-precio-de-la-gasolina/
  - /es/guias/icms-sobre-combustibles/
---

La ANP, el regulador brasileño del petróleo y los combustibles (Agencia
Nacional del Petróleo, Gas Natural y Biocombustibles), hace un seguimiento
semanal de los precios de los combustibles en Brasil. Esta guía explica qué
abarca la encuesta, cómo usar sus datos históricos y por qué los precios
pueden cambiar entre visitas.

## Qué es la encuesta de precios de la ANP

La [Encuesta de Precios de Combustibles de la ANP](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/precos-revenda-e-de-distribuicao-combustiveis/informacoes-levantamento-de-precos-de-combustiveis)
registra los precios de venta al público. Abarca combustibles automotores
como la gasolina (nafta) regular y con aditivos, el etanol hidratado (E100),
el diésel (gasoil) y el gas natural vehicular, GNV (GNC). Una empresa
contratada por la ANP recopila los precios mediante visitas a las estaciones.

## Metodología de muestreo y cobertura

El [Informe de Gestión 2025 de la ANP](https://www.gov.br/anp/pt-br/acesso-a-informacao/transparencia/rg/relatorio-gestao-2025.pdf)
registra más de 40 mil estaciones de servicio de combustibles automotores en
Brasil. La encuesta semanal visita una muestra de esas estaciones.

- Para seleccionar las localidades, se tienen en cuenta factores como los
  ingresos, la población, el parque vehicular y el número de estaciones.
- Según la [metodología publicada por la ANP](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/precos-revenda-e-de-distribuicao-combustiveis/arquivos-metodologia/metodologia-coleta.pdf),
  el equipo de la encuesta sortea las estaciones que visitará cada semana en
  cada municipio encuestado, teniendo en cuenta la cobertura geográfica.

## Periodicidad y desfase temporal

Los datos de campo de la ANP normalmente se recolectan a lo largo de los días
hábiles de una semana. Después de la recolección, los datos pasan por etapas de
consolidación, crítica estadística y validación antes de publicarse en los
informes oficiales y en el portal de datos abiertos del gobierno.

::: atencao
**Atención al desfase:** cuando se publica un informe de la ANP (generalmente
al final de la semana o al inicio de la semana siguiente), los precios
registrados se refieren a la semana anterior, la de la recolección. Si una
estación cambió su valor el martes por la tarde, el dato oficial de la ANP no
reflejará ese cambio de inmediato.
:::

## Referencia ANP y colaboración comunitaria

En el proyecto **abastevo**, tratamos la encuesta de la ANP con total claridad
sobre su naturaleza:

- **Referencia regulatoria:** los datos de la ANP se muestran con la fecha
  exacta de la semana de referencia a la que pertenecen, para que el conductor
  tenga un parámetro oficial confiable del promedio de la región.
- **Realidad en tiempo real:** para saber el precio que está hoy en el
  surtidor, el proyecto cuenta con observaciones colaborativas enviadas por la
  comunidad de conductores en la app, con la fecha y la hora exactas de cada
  registro.

## Acceso a los datos abiertos e independencia

Los datos consolidados de la ANP son públicos y accesibles a cualquier
ciudadano en el portal oficial del gobierno federal, en
[gov.br/anp](https://www.gov.br/anp), y en el Portal Brasileño de Datos
Abiertos.

abastevo es una iniciativa independiente y de código bajo la licencia de
atribución visible Abastevo. **No tenemos ninguna afiliación gubernamental ni
vínculo institucional con la ANP**: solo usamos los datos públicos que la
agencia pone a disposición, de conformidad con la legislación de datos
abiertos.

## En la app

La app Android de abastevo está en desarrollo. Las capturas de la guía en
portugués son reales, se tomaron el 17 de noviembre de 2026 y muestran la
referencia de la ANP de la semana indicada, no el precio en el surtidor. Las capturas muestran la interfaz en portugués. Para ver
la encuesta semanal dentro de la app:

1. En la pantalla inicial, la línea "Referencia ANP", debajo
   de "Precios en tu ciudad", informa el período de la
   encuesta. Cada tarjeta trae el sello "ANP" y el número de estaciones
   encuestadas.
2. Toca la semana junto al nombre de la ciudad para abrir "Semana de encuesta". El resumen "Cada semana incluye"
   enumera lo que viene en cada semana: precios promedio semanales y precios
   por estación minorista.
3. Toca "Descargar" para guardar una semana anterior en el dispositivo
   y compararla con la actual.

La captura del paso 1 muestra la referencia de la ANP en São Paulo, SP, en la
semana del 8 al 14 de noviembre de 2026, con tarjetas de etanol, gasolina
regular y gasolina con aditivos. La del paso 2 muestra la pantalla inicial en
Sorriso, MT, con el panel de la semana de encuesta abierto. La del paso 3
muestra la semana del 8 al 14 de noviembre de 2026 marcada como actual y
descargada, y el botón de descarga en las semanas anteriores.

El paso a paso completo está en los tutoriales
[Consultar la tabla de la ANP](/como-usar/precos-anp/) (en portugués) y
[Cambiar o descargar una semana](/como-usar/semanas-anp/) (en portugués).
