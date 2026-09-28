package com.anpfuel.domain.valueobject

/**
 * ANP data presence for a catalog municipality relative to the active survey week.
 */
enum class DataAvailability {
    /** Municipality has average prices for the selected [SurveyWeek]. */
    HAS_DATA,

    /** Municipality appears in ANP history but has no rows for the selected week. */
    NO_DATA_THIS_WEEK,

    /** Municipality exists in the IBGE catalog but has never appeared in ANP imports. */
    NEVER_IN_ANP,
}
