package com.anpfuel.domain.quality

import java.time.LocalDate
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class QualityReportCommandTest {
    private val today=LocalDate.of(2026,10,18)
    private val id="eeeeeeee-5555-4555-8555-000000000001"
    private fun command()=QualityReportCommand(id,id,"SUBMIT",0,"EXPERIENCE",id,"ETHANOL","2026-10-18","OTHER","Synthetic claim")
    @Test fun `unverified experience and unicode limit follow backend contract`() {
        command().validate(today)
        command().copy(text="😀".repeat(1000)).validate(today)
        assertThrows(IllegalArgumentException::class.java){command().copy(text="😀".repeat(1001)).validate(today)}
        for(text in listOf(" ","private\u0000control","\uD800")) assertThrows(IllegalArgumentException::class.java){command().copy(text=text).validate(today)}
    }
    @Test fun `future invalid day missing link and stale action fail closed`() {
        for(c in listOf(command().copy(occurredOn="2026-10-19"),command().copy(occurredOn="2026-02-30"),command().copy(kind="LINK_REVIEW"),command().copy(officialRevisionId=id),command().copy(action="EDIT"),command().copy(fuel="UNKNOWN"))) {
            assertThrows(IllegalArgumentException::class.java){c.validate(today)}
        }
        command().copy(kind="LINK_REVIEW",officialRevisionId=id,sampleRef="sample:checked").validate(today)
    }
    @Test fun `withdrawal and appeal cannot carry mutable report fields`() {
        QualityReportCommand(id,id,"WITHDRAW",2).validate(today)
        QualityReportCommand(id,id,"APPEAL",2,text="Please review").validate(today)
        for(c in listOf(QualityReportCommand(id,id,"WITHDRAW",2,text="unexpected"),QualityReportCommand(id,id,"APPEAL",2),QualityReportCommand(id,id,"APPEAL",2,text="review",stationId=id))) {
            assertThrows(IllegalArgumentException::class.java){c.validate(today)}
        }
    }
}
