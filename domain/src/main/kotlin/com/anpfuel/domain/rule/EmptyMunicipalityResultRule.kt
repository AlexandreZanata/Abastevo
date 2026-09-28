package com.anpfuel.domain.rule

/**
 * BR-010 — A valid municipality with no rows for the selected week is an empty result, not an error.
 */
object EmptyMunicipalityResultRule {

    fun shouldReturnEmptyList(rowCount: Int): Boolean = rowCount == 0

    fun isError(rowCount: Int): Boolean = false
}
