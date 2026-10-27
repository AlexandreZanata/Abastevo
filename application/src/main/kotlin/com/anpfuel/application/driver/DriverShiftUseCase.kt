package com.anpfuel.application.driver

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.driver.*

/** The storage implementation must serialize owner-wide changes atomically.
 * It may not publish the candidate when the transform throws. Durable adapters
 * belong to DRV-08/14; this port does not claim cross-device offline uniqueness.
 */
interface DriverShiftAuthority {
    suspend fun transact(owner: String, transform: (DriverShift?) -> DriverShift): DriverShift
}
class DriverShiftUseCase(private val authority: DriverShiftAuthority, private val sessions: AuthSessionStore, private val origin: String) {
    suspend fun invoke(initial: DriverShift, command: DriverShiftCommand, policy: DriverClockPolicy): DriverShift {
        val owner = sessions.load()?.accountId ?: "local"
        require(initial.owner == owner && initial.origin == origin)
        val result = authority.transact(owner) { current ->
            check((sessions.load()?.accountId ?: "local") == owner) { "driver-owner-changed" }
            check(current == null || current.owner == owner) { "driver-owner-conflict" }
            val same = current?.takeIf { it.id == initial.id }
            if (same == null) {
                check(current == null || current.state == DriverShiftState.CLOSED) { "driver-active-shift-conflict" }
                require(initial.state == DriverShiftState.READY && initial.version == 0L)
            } else check(same.origin == origin) { "driver-origin-conflict" }
            (same ?: initial).apply(command, policy).shift
        }
        check((sessions.load()?.accountId ?: "local") == owner) { "driver-owner-changed" }
        return result
    }
}
