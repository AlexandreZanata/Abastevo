package com.anpfuel.domain.quality

import java.time.LocalDate

/** User claims stay private and separate from official sample judgments. */
data class QualityReportCommand(
    val commandId:String,
    val reportId:String,
    val action:String,
    val expectedRevision:Int,
    val kind:String="",
    val stationId:String="",
    val fuel:String="",
    val occurredOn:String="",
    val symptom:String="",
    val text:String="",
    val officialRevisionId:String="",
    val sampleRef:String="",
) {
    fun validate(today:LocalDate) {
        require(qualityReportId(reportId) && qualityReportId(commandId))
        require(expectedRevision in 0 until Int.MAX_VALUE)
        if(action=="WITHDRAW" || action=="APPEAL") {
            require(expectedRevision>=1 && listOf(kind,stationId,fuel,occurredOn,symptom,officialRevisionId,sampleRef).all {it.isEmpty()})
            if(action=="WITHDRAW") require(text.isEmpty()) else validateText(text)
            return
        }
        require(action in setOf("SUBMIT","EDIT"))
        require(if(action=="SUBMIT")expectedRevision==0 else expectedRevision>=1)
        require(kind in setOf("EXPERIENCE","LINK_REVIEW") && qualityReportId(stationId) && fuel in QUALITY_FUELS)
        require(runCatching {LocalDate.parse(occurredOn).let {it.toString()==occurredOn && !it.isAfter(today)}}.getOrDefault(false))
        require(symptom in setOf("STARTING","PERFORMANCE","CONSUMPTION","OTHER"))
        validateText(text)
        require(officialRevisionId.isEmpty()==sampleRef.isEmpty())
        require(kind!="LINK_REVIEW" || sampleRef.isNotEmpty())
        if(officialRevisionId.isNotEmpty()) require(qualityReportId(officialRevisionId) && sampleRef.toByteArray(Charsets.UTF_8).size<=128 && validUnicode(sampleRef))
    }
}

data class QualityReportReceipt(val reportId:String,val revision:Int,val status:String)
data class QualityReport(
    val id:String,val revision:Int,val kind:String,val stationId:String,val fuel:String,
    val occurredOn:String,val symptom:String,val text:String,val officialRevisionId:String,
    val sampleRef:String,val status:String,val evidenceLevel:String,val reviewReason:String,
)
data class QualityReportPage(val items:List<QualityReport>,val nextAfterId:String?)
enum class QualityReportFailure { AUTH_REQUIRED, OWNER_CHANGED, INVALID, CONFLICT, NOT_FOUND, QUOTA, UNAVAILABLE }
class QualityReportException(val kind:QualityReportFailure):Exception("quality report unavailable or refused")

fun qualityReportId(value:String):Boolean=Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}").matches(value)
private fun validUnicode(value:String):Boolean {
    var index=0
    while(index<value.length) {
        val c=value[index++]
        if(Character.isHighSurrogate(c)) {
            if(index==value.length || !Character.isLowSurrogate(value[index++]))return false
        } else if(Character.isLowSurrogate(c))return false
    }
    return true
}
private fun validateText(value:String) {
    require(value.isNotBlank() && validUnicode(value) && value.codePointCount(0,value.length)<=1000)
    require(value.codePoints().allMatch {it==10 || it==9 || !Character.isISOControl(it)})
}
