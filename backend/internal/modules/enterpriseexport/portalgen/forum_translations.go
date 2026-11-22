package portalgen

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type forumTranslation struct{ title, excerpt, body string }

// These owner-authored posts were translated and reviewed against the public
// Portuguese versions on 2026-11-22. A change to the source title or body
// expires the review. Routes, timestamps, sources and authors always come
// from the current publication-eligible export.
var reviewedForumPosts = map[string]struct {
	source string
	copies map[string]forumTranslation
}{
	"3c8b784f": {
		source: "354b8c2cd71f4915213e45b3584a2fcf71ef9b16e6243237ac9d158d7bed8998",
		copies: map[string]forumTranslation{
			"en": {
				title:   "Ethanol or gasoline? How to compare before filling up",
				excerpt: "Compare cost per kilometer using your own flex-fuel car’s fuel economy and the prices at the pump.",
				body: `If you drive a flex-fuel car, you probably ask the same question nearly every time you stop for fuel: will ethanol or gasoline cost less? What matters is both the price of a liter and how far it takes your car.

Why the cheaper liter does not always save money

Ethanol contains less energy per liter than gasoline. In practice, the same car usually travels fewer kilometers on one liter of ethanol. Ethanol only saves money when its lower price makes up for the difference in fuel economy.

The 70% rule of thumb

The most common shortcut is to divide the price of ethanol by the price of gasoline:

- A result of 0.70 or less: ethanol tends to cost less.
- A result above 0.70: gasoline tends to cost less.

For example, ethanol at R$ 3.88 and gasoline at R$ 6.59 gives a ratio of about 0.59. Ethanol is the cheaper choice in that example.

How to calculate it for your own car

The 70% figure is an average. Many modern flex-fuel engines get a little better mileage on ethanol, and your driving style, traffic and road conditions also affect the result. For a closer estimate, use your car’s actual fuel economy:

1. Measure how many kilometers per liter your car travels with each fuel. Use the trip computer or compare full-tank fill-ups.
2. Divide fuel economy on ethanol by fuel economy on gasoline to get your cutoff. For example, 8.4 km/L on ethanol divided by 12 km/L on gasoline gives 0.70.
3. Divide the price of ethanol by the price of gasoline. If the result is below your cutoff, choose ethanol.

For a more direct comparison, divide the price per liter by the kilometers that liter takes you. The fuel with the lower cost per kilometer is cheaper to drive on.

Other factors to consider

- On a cold start, some cars get worse mileage on ethanol for the first few minutes.
- On long trips, gasoline gets you farther on a tank, which can save a stop.
- Prices vary by station and from week to week. Today’s result might be different next month.

How Abastevo can help

In Explore, you can compare ethanol and gasoline prices in your city side by side, including the ANP survey average and the stations surveyed. In Community, you can see prices submitted by drivers. Add your vehicle to see the estimated cost of a full tank and compare before leaving home.

If you see a different price at the station, share it in Community. Every reported price helps someone else make the comparison.`,
			},
			"es": {
				title:   "¿Etanol o gasolina? Cómo hacer la cuenta antes de cargar",
				excerpt: "Compara el costo por kilómetro con los precios del surtidor y el consumo real de tu vehículo flex.",
				body: `Si tienes un auto flex, seguramente te haces la misma pregunta casi siempre que paras a cargar: ¿conviene el etanol o la gasolina? Importan tanto el precio por litro como la distancia que recorre tu auto con ese litro.

Por qué el litro más barato no siempre resulta más económico

El etanol aporta menos energía por litro que la gasolina. En la práctica, el mismo auto suele recorrer menos kilómetros con un litro de etanol. Por eso el etanol solo conviene cuando su precio compensa esa diferencia de rendimiento.

La regla práctica del 70 %

La cuenta más conocida consiste en dividir el precio del etanol por el de la gasolina:

- Resultado de hasta 0,70: el etanol suele convenir.
- Resultado superior a 0,70: la gasolina suele convenir.

Por ejemplo, con etanol a R$ 3,88 y gasolina a R$ 6,59, el resultado es aproximadamente 0,59. En ese caso, el etanol sale más barato.

Cómo hacer la cuenta para tu auto

El 70 % es un promedio. Muchos motores flex modernos rinden un poco más con etanol y el resultado también depende de cómo conduces, del tráfico y del camino. Para afinar la cuenta, usa el rendimiento real de tu auto:

1. Mide cuántos kilómetros por litro recorre con cada combustible. Puedes usar la computadora de a bordo o medir entre cargas con el tanque lleno.
2. Divide el rendimiento con etanol por el rendimiento con gasolina. Ese es tu límite. Por ejemplo, 8,4 km/l con etanol dividido por 12 km/l con gasolina da 0,70.
3. Divide el precio del etanol por el precio de la gasolina. Si el resultado queda por debajo de tu límite, elige etanol.

Para comparar de forma más directa, divide el precio por litro por los kilómetros que puedes recorrer con ese litro. El combustible con el menor costo por kilómetro resulta más económico.

Otros factores para elegir

- En un arranque en frío, algunos autos rinden menos con etanol durante los primeros minutos.
- En viajes largos, la gasolina da más autonomía por tanque y puede ahorrar una parada.
- Los precios cambian según la estación y de una semana a otra. La cuenta de hoy puede ser distinta el mes que viene.

Cómo ayuda Abastevo

En Explorar puedes comparar los precios de etanol y gasolina de tu ciudad, con el promedio de la encuesta de la ANP y las estaciones relevadas. En Comunidad ves los precios compartidos por otros conductores. Registra tu vehículo para estimar cuánto cuesta llenar el tanque y comparar antes de salir.

Si encuentras un precio distinto en la estación, compártelo en Comunidad. Cada precio enviado ayuda a otra persona a hacer la cuenta.`,
			},
		},
	},
	"44093819": {
		source: "d05cf376f31c17313bb644b3d15efeeed4c1514c6cc8c4d78df10cec8da5068d",
		copies: map[string]forumTranslation{
			"en": {
				title:   "How ANP’s weekly survey works and how to read Abastevo prices",
				excerpt: "Understand the ANP weekly fuel price sample, check the collection date, and distinguish official observations from community reports.",
				body: `Official prices in Abastevo come from the Fuel Price Survey run by ANP, Brazil’s National Agency of Petroleum, Natural Gas and Biofuels. Knowing how the survey works helps you understand what each number represents.

What the weekly survey covers

Each week, ANP collects prices at a sample of fuel retailers in municipalities across Brazil. It covers automotive fuels such as gasoline, ethanol, diesel and CNG, as well as cooking gas (LPG sold in 13 kg cylinders). Each survey week runs from Sunday to Saturday.

ANP publishes two types of information based on the survey:

- Weekly average prices for Brazil, its regions, states and surveyed municipalities.
- Prices recorded at each fuel retailer in the sample.

Abastevo downloads both publications and shows your city's average and the list of surveyed stations.

Why some stations do not appear

The survey samples stations. It does not visit every station in every city, and some municipalities are not included every week. This means:

- The average captures that week; it does not show the exact price at every station.
- A station you know might be missing from that week’s list.
- A station may change its prices between surveys.

How to select a survey week

Go to Profile and choose Survey week to see the weeks ANP has published. The most recent week is marked Current. You can download earlier weeks to compare price changes. Downloaded weeks remain available offline.

Official prices and community prices

Abastevo keeps the two sources separate:

- ANP: official weekly survey data, with the collection date.
- Community: prices reported by drivers, with the submission time.

Community prices can cover stations outside the sample and changes that happened after a survey. They supplement the official data, and Abastevo never combines the two sources in one average.

How to make the most of the numbers

- Check the date. A price from three weeks ago may be out of date.
- Use the average to see whether a price is high or low for your city.
- Compare the list of stations and the community prices to decide where to fill up.

Did you see a different price at the station? Share it in Community. The more drivers contribute, the fuller the picture of prices in your city.`,
			},
			"es": {
				title:   "Cómo funciona la encuesta semanal de la ANP y cómo leer los precios en Abastevo",
				excerpt: "Entiende la muestra semanal de la ANP, comprueba la fecha de recolección y distingue los datos oficiales de los precios de la comunidad.",
				body: `Los precios oficiales que ves en Abastevo vienen del relevamiento de precios de combustibles de la ANP, la Agencia Nacional del Petróleo, Gas Natural y Biocombustibles de Brasil. Entender cómo funciona la encuesta ayuda a interpretar cada cifra.

Qué cubre la encuesta semanal

Cada semana, la ANP recoge los precios de una muestra de estaciones de servicio en municipios de todo Brasil. La encuesta abarca combustibles como gasolina, etanol, diésel y GNV, además del gas de cocina (GLP en cilindros de 13 kg). Cada semana de encuesta va de domingo a sábado.

Con esos datos, la ANP publica dos tipos de información:

- Precios medios semanales para Brasil, sus regiones, estados y municipios relevados.
- Precios observados en cada estación de la muestra.

Abastevo descarga las dos publicaciones y muestra el promedio de tu ciudad y la lista de estaciones relevadas.

Por qué no aparecen todas las estaciones

La encuesta se realiza por muestreo. No visita todas las estaciones de todas las ciudades y algunos municipios quedan fuera en ciertas semanas. Por eso:

- El promedio refleja esa semana; no muestra el precio exacto de cada estación.
- Una estación que conoces puede quedar fuera de la lista de esa semana.
- Entre encuestas, una estación puede cambiar sus precios.

Cómo elegir la semana

En Perfil, abre Semana de la encuesta para ver las semanas publicadas. La más reciente figura como Actual. Puedes descargar semanas anteriores para comparar la evolución de los precios y consultar las que ya descargaste sin conexión.

Precios oficiales y de la comunidad

Abastevo mantiene las dos fuentes separadas:

- ANP: datos oficiales de la encuesta semanal, con la fecha de recolección.
- Comunidad: precios enviados por conductores, con la hora del envío.

Los precios de la comunidad ayudan a cubrir estaciones que no están en la muestra y cambios posteriores a la encuesta. Complementan los datos oficiales y Abastevo nunca mezcla ambas fuentes en un mismo promedio.

Consejos para aprovechar los datos

- Comprueba la fecha. Un precio de hace tres semanas puede estar desactualizado.
- Usa el promedio para saber si un precio es alto o bajo para tu ciudad.
- Compara la lista de estaciones con los precios de la comunidad para decidir dónde cargar.

¿Viste un precio distinto en la estación? Compártelo en Comunidad. Cuantos más conductores participen, más completa será la información de tu ciudad.`,
			},
		},
	},
}

func reviewedForumCopy(post *forumPost, code string) (forumContent, bool) {
	review, ok := reviewedForumPosts[post.ID8]
	if !ok || post.Author != forumEditorialLabel {
		return forumContent{}, false
	}
	copy, ok := review.copies[code]
	if !ok {
		return forumContent{}, false
	}
	source := strings.Join(strings.Fields(post.Row.Title), " ") + "\x00" + forumPlainText(post.Blocks)
	digest := sha256.Sum256([]byte(source))
	if hex.EncodeToString(digest[:]) != review.source {
		return forumContent{}, false
	}
	return forumContent{Title: copy.title, Excerpt: copy.excerpt, Language: code, Blocks: parseForumBody(copy.body)}, true
}
