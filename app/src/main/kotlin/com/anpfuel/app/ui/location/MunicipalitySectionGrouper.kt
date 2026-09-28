package com.anpfuel.app.ui.location
import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import java.text.Normalizer
data class MunicipalityListSection(val letter: Char, val items: List<CatalogMunicipalityItem>)
internal fun groupMunicipalitiesBySectionLetter(municipalities: List<CatalogMunicipalityItem>) =
    municipalities.groupBy { sectionLetter(it.municipality) }.toSortedMap().map { (l, i) -> MunicipalityListSection(l, i) }
internal fun sectionLetter(municipality: String): Char {
    val n = Normalizer.normalize(municipality, Normalizer.Form.NFD).replace(Regex("\\p{Mn}+"), "")
    val f = n.firstOrNull()?.uppercaseChar() ?: return '#'
    return if (f in 'A'..'Z') f else '#'
}
