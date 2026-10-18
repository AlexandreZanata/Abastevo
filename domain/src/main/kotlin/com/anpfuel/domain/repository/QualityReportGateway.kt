package com.anpfuel.domain.repository

import com.anpfuel.domain.quality.*

/** Scope binds origin, free account and device identity; never contains credentials. */
interface QualityReportGateway {
    suspend fun owner():String
    fun currentOwner():String?
    suspend fun execute(owner:String,command:QualityReportCommand):QualityReportReceipt
    suspend fun get(owner:String,reportId:String):QualityReport
    suspend fun list(owner:String,afterId:String?=null):QualityReportPage
}
