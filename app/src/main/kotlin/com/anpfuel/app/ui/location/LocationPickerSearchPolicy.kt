package com.anpfuel.app.ui.location

/**
 * MEX-06 measured search policy for the location picker.
 *
 * Local state/city filtering runs synchronously in composition over
 * already-loaded bounded lists (27 states; one state's municipalities)
 * with a precomputed normalized index, so keystrokes produce immediate
 * results with no artificial delay. The host JVM benchmark in
 * `LocationPickerFilterBenchmarkTest` pins the measured cost; native
 * keystroke-to-visible timing stays with MEX-06 device acceptance and the
 * deferred device matrix.
 *
 * Async catalog loads are not debounced either: they are one-shot per user
 * action and fenced by generation in [LocationPickerViewModel], so an
 * obsolete response can neither delay nor replace the current one. Change
 * this constant only with a new measurement that justifies the added
 * latency, per B-BR-MEX-19 (no unmeasured "instant" promise in reverse:
 * no unmeasured delay).
 */
internal object LocationPickerSearchPolicy {

    const val QUERY_DEBOUNCE_MS = 0L
}
