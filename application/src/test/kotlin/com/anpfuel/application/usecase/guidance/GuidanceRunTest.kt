package com.anpfuel.application.usecase.guidance

import com.anpfuel.domain.routing.NavigationFix
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class GuidanceRunTest {
    private class Runtime:GuidanceRuntime {
        val events=mutableListOf<String>();var locationFailure=false;var stopFailure=false;var voiceFailure=false;var nativeReady=true
        override fun startNative():Boolean {events+="native-start";return nativeReady}
        override fun stopNative(){events+="native-stop"}
        override fun startLocation(){events+="gps-start";if(locationFailure)throw SecurityException("permission revoked")}
        override fun stopLocation(){events+="gps-stop";if(stopFailure)throw IllegalStateException("provider unavailable")}
        override fun startVoice(){events+="voice-start";if(voiceFailure)throw IllegalStateException("no engine")}
        override fun pauseVoice(){events+="voice-pause"}
        override fun stopVoice(){events+="voice-stop"}
    }
    @Test fun `partial location start failure releases every resource and repeated stop is idempotent`() {
        val rt=Runtime().apply {locationFailure=true;stopFailure=true}
        val run=GuidanceRun("owner",{"owner"},rt)
        assertFalse(run.start());run.stop()
        assertEquals(listOf("native-start","voice-start","gps-start","gps-stop","voice-stop","native-stop"),rt.events)
        assertFalse(run.active)
    }
    @Test fun `voice absence preserves textual guidance and owner switch stops before another fix`() {
        val rt=Runtime().apply {voiceFailure=true};var owner="owner"
        val run=GuidanceRun(owner,{owner},rt);assertTrue(run.start())
        assertTrue(run.accept(NavigationFix(-25.43,-49.27,5f,1,100),100))
        owner="different";assertFalse(run.accept(NavigationFix(-25.43,-49.27,5f,1,101),101))
        assertFalse(run.active);assertEquals(1,rt.events.count {it=="gps-stop"})
    }
    @Test fun `stale poor or future GPS pauses speech without advancing or losing the subscription`() {
        val rt=Runtime();val run=GuidanceRun("owner",{"owner"},rt);assertTrue(run.start())
        assertFalse(run.accept(NavigationFix(-25.43,-49.27,500f,1,100),100))
        assertFalse(run.accept(NavigationFix(-25.43,-49.27,5f,1,100),16_000_000_101))
        assertTrue(run.active);assertEquals(2,rt.events.count {it=="voice-pause"})
        run.stop();assertEquals(1,rt.events.count {it=="native-stop"})
    }
    @Test fun `foreign launch and invalid native preview never start location`() {
        val rt=Runtime();val foreign=GuidanceRun("old",{"new"},rt);assertFalse(foreign.start());assertTrue(rt.events.isEmpty())
        rt.nativeReady=false
        val refused=GuidanceRun("new",{"new"},rt);assertFalse(refused.start());assertFalse(rt.events.contains("gps-start"))
    }
}
