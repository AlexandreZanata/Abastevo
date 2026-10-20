package com.anpfuel.application.port

/** Durable blocking and key-erasure result; false leaves repair owed. */
fun interface QualityReportLocalErasure {
    suspend fun eraseOwner(ownerScope:String):Boolean
}
