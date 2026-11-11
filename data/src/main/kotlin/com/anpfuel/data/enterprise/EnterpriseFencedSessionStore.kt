package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth

/** Fence before durable session replacement/clear, including failed writes. */
class EnterpriseFencedSessionStore(
    private val delegate: AuthSessionStore,
    private val fence: EnterpriseSessionFence,
) : AuthSessionStore {
    override fun load(): PortableAuth.Session? = delegate.load()
    override fun save(session: PortableAuth.Session) = fence.access {
        val old = delegate.load()
        if (old?.accountId != session.accountId || old?.familyId != session.familyId) fence.lock()
        delegate.save(session)
    }
    override fun clear() = fence.access {
        fence.lock()
        delegate.clear()
    }
}
