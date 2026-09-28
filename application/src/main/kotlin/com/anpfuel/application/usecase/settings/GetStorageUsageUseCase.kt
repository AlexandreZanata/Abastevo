package com.anpfuel.application.usecase.settings

import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.repository.StorageStatsRepository

class GetStorageUsageUseCase(
    private val storageStatsRepository: StorageStatsRepository,
) {

    suspend operator fun invoke(): StorageUsage =
        storageStatsRepository.getStorageUsage()
}
