package com.anpfuel.application.usecase.privacy
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.TrainingConsentLocalState
import io.mockk.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
class ResumeTrainingRevocationTest {
 private val scope=ContributionScope("account:a:key","https://test.invalid")
 private val caller=TrainingConsentCaller("a",scope)
 private val local=mockk<TrainingConsentLocalStore>()
 private val use=mockk<TrainingConsentUseCase>()
 private val resume=ResumeTrainingRevocation(local,use)
 @Test fun `guests unscoped owners and pending ON never dispatch`() = runTest {
  resume(null);resume(TrainingConsentCaller("a"))
  every {local.load(scope)} returns TrainingConsentLocalState()
  resume(caller)
  every {local.load(scope)} returns TrainingConsentLocalState().grant("on","v1","digest",0)
  resume(caller)
  coVerify(exactly=0) {use.syncPending(any())}
 }
 @Test fun `original scoped OFF resumes and cancellation stays visible`() = runTest {
  every {local.load(scope)} returns TrainingConsentLocalState().revoke("off")
  coEvery {use.syncPending(caller)} returns TrainingConsentOutcome.RevocationPending
  resume(caller);coVerify(exactly=1) {use.syncPending(caller)}
  coEvery {use.syncPending(caller)} throws CancellationException()
  try {resume(caller);fail<Unit>("cancel swallowed")} catch(_:CancellationException) {}
 }
}
