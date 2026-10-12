package com.anpfuel.domain.repository

/** The receipt binds the original UTF-8 bytes, including its final newline. */
data class TrainingTermsDocument(val version:String,val digest:String,val text:String)

/** Review is a separate, ephemeral action; neither reading nor a saved checkbox grants consent. */
data class TrainingConsentReview(val owner:String,val version:String,val digest:String,val read:Boolean=false,val checked:Boolean=false) {
    val canConfirm:Boolean get()=owner.isNotBlank() && version.isNotBlank() && digest.matches(Regex("[a-f0-9]{64}")) && read && checked
    fun finishReading()=copy(read=true,checked=false)
    fun check(value:Boolean)=copy(checked=read && value)
}
