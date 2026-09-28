package com.anpfuel.data.local.importing

import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.entity.ImportAuditLogEntity
import com.anpfuel.domain.valueobject.DomainId

class ImportAuditLogger(
    private val importAuditLogDao: ImportAuditLogDao,
) {

    suspend fun append(
        action: ImportAuditAction,
        surveyWeekId: String? = null,
        detail: String? = null,
        occurredAt: Long = System.currentTimeMillis(),
    ) {
        importAuditLogDao.insert(
            ImportAuditLogEntity(
                id = DomainId.generate().value,
                surveyWeekId = surveyWeekId,
                action = action.name,
                detail = detail,
                occurredAt = occurredAt,
            ),
        )
    }
}
