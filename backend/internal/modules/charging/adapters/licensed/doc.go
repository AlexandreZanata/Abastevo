// Package licensed owns the licensed-operator read port (EE-17).
//
// Without authorized access every read fails closed with ACCESS_PENDING:
// no signup, contact, payment, credential or private API exists (EE-01
// register, optional_operators). The contract below is executable —
// config validation, access resolution and the full fault taxonomy run
// in tests — but no live operator feed is claimed and no provider path
// is wired into jobs or HTTP. Binding a real partner (pinned OCPI
// version/modules or documented API plus credential) needs its own
// authorized review; until then the default client has no transport and
// every read refuses with route-unpinned.
package licensed
