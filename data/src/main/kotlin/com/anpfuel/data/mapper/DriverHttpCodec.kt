package com.anpfuel.data.mapper

import org.json.JSONObject

data class DriverReceipt(
    val id: String,
    val revision: Long,
    val epoch: Long,
    val replayed: Boolean,
)

data class DriverApiError(val code: String, val message: String)

/** DRV-14 strict decoding of the shared Go HTTP wire fixtures. */
object DriverHttpCodec {
    fun receipt(body: String): DriverReceipt {
        val obj = JSONObject(body)
        val id = obj.getString("id")
        val revision = obj.getLong("revision")
        val epoch = obj.getLong("epoch")
        val replayed = obj.getBoolean("replayed")
        require(id.isNotBlank() && revision >= 1 && epoch >= 0)
        return DriverReceipt(id, revision, epoch, replayed)
    }

    fun error(status: Int, body: String): DriverApiError {
        require(status == 401 || status == 409)
        val error = JSONObject(body).getJSONObject("error")
        val code = error.getString("code")
        val message = error.getString("message")
        require(code.isNotBlank() && message.isNotBlank())
        return DriverApiError(code, message)
    }
}
