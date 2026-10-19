package com.anpfuel.domain.repository

import com.anpfuel.domain.quality.*

/** Scope binds origin, free account, device identity and login family; never contains credentials. */
interface QualityReportGateway {
    suspend fun owner():String
    fun currentOwner():String?
    suspend fun pending(owner:String):QualityReportCommand?=null
    suspend fun execute(owner:String,command:QualityReportCommand):QualityReportReceipt
    suspend fun get(owner:String,reportId:String):QualityReport
    suspend fun list(owner:String,afterId:String?=null):QualityReportPage
}
