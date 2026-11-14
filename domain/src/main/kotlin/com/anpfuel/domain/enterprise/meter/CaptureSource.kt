package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.EnterprisePartition

enum class CaptureKind { ENROLLED_DEVICE, WEB_SESSION;
    companion object {
        fun parse(raw: String): CaptureKind = entries.firstOrNull { it.name == raw } ?: throw MeterDomainException()
    }
}

/** Historic supplied locator only. It contains no token or live authority proof. */
data class CaptureSource(val organizationId: String, val kind: CaptureKind, val id: String) {
    init {
        try { EnterprisePartition.requireId(organizationId) }
        catch (_: IllegalArgumentException) { throw MeterDomainException(MeterFailure.SCOPE) }
        try { EnterprisePartition.requireId(id) }
        catch (_: IllegalArgumentException) { throw MeterDomainException() }
    }
    override fun toString(): String = "EnterpriseMeterCaptureSource(redacted)"
}
