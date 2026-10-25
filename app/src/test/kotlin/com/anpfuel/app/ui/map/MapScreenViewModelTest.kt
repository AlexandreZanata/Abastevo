package com.anpfuel.app.ui.map

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class MapScreenViewModelTest {
    @Test fun `default source is ANP with no package banner`() {
        val vm = MapScreenViewModel()
        assertEquals(MapPriceSource.ANP, vm.ui.value.source)
        assertEquals(true, vm.ui.value.noPackage)
        assertNull(vm.ui.value.error)
    }

    @Test fun `source switch is explicit`() {
        val vm = MapScreenViewModel()
        vm.selectSource(MapPriceSource.COMMUNITY)
        assertEquals(MapPriceSource.COMMUNITY, vm.ui.value.source)
        vm.selectSource(MapPriceSource.ANP)
        assertEquals(MapPriceSource.ANP, vm.ui.value.source)
    }

    @Test fun `retry clears error`() {
        val vm = MapScreenViewModel()
        vm.retry()
        assertNull(vm.ui.value.error)
    }
}
