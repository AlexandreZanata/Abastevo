package com.anpfuel.data.local.quality

import com.anpfuel.domain.quality.QualityReportCommand
import java.time.Instant
import javax.crypto.KeyGenerator
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class QualityReportSealTest {
    private val now=Instant.parse("2026-10-18T00:00:00Z").toEpochMilli()
    private val id="eeeeeeee-5555-4555-8555-000000000001"
    private fun key()=KeyGenerator.getInstance("AES").apply {init(256)}.generateKey()
    private val command=QualityReportCommand(id,id,"SUBMIT",0,"EXPERIENCE",id,"ETHANOL","2020-01-01","OTHER","Synthetic private claim 😀")
    @Test fun `encrypted restart representation retains exact intent without plaintext`() {
        val key=key();val row=QualityReportSeal.freeze("a".repeat(64),command,key,now,now+10000)
        assertFalse(row.sealedCommand.contains(command.text))
        assertFalse(row.sealedCommand.contains(command.reportId))
        assertEquals(command,QualityReportSeal.open(row.copy(),key,now+1))
        assertEquals(row.payloadHash,QualityReportSeal.freeze(row.ownerKey,command,key,now,now+10000).payloadHash)
    }
    @Test fun `foreign key owner stamp command hash and tamper refuse private data`() {
        val key=key();val row=QualityReportSeal.freeze("a".repeat(64),command,key,now,now+10000)
        assertNull(QualityReportSeal.open(row,key(),now))
        for(bad in listOf(row.copy(ownerKey="b".repeat(64)),row.copy(createdAt=now-1),row.copy(payloadHash="b".repeat(64)),row.copy(commandId="other"),row.copy(sealedCommand=row.sealedCommand.replace("\"v\":1","\"v\":2"))))assertNull(QualityReportSeal.open(bad,key,now))
        assertNull(QualityReportSeal.open(row,key,now-1))
        assertNull(QualityReportSeal.open(row,key,now+10000))
    }
}
