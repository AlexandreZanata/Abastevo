package com.anpfuel.domain.enterprise.sync

import java.net.URI
import java.net.URISyntaxException

/** A captured canonical origin is validated verbatim; no DNS or normalization occurs. */
internal fun canonicalOrigin(raw: String): String {
    val uri = try { URI(raw) } catch (_: URISyntaxException) { throw invalid() }
    if (uri.scheme != "https" || uri.rawUserInfo != null || uri.rawQuery != null || uri.rawFragment != null ||
        !uri.rawPath.isNullOrEmpty() || uri.port == 443 || uri.port != -1 && uri.port !in 1..65535) throw invalid()
    val host = uri.host ?: throw invalid()
    if (host.any { it !in '!'..'~' || it in 'A'..'Z' || it == '%' }) throw invalid()
    val port = if (uri.port == -1) "" else ":${uri.port}"
    if (raw != "https://$host$port") throw invalid()
    if (host.startsWith('[')) {
        // URI validates numeric IPv6 syntax locally. Zone identifiers are rejected above.
        val literal = host.substring(1, host.length - 1)
        if ('.' in literal && !ipv4(literal.substringAfterLast(':'))) throw invalid()
    } else {
        val dns = host.removeSuffix(".")
        if (dns.isEmpty() || dns.length > 253 || dns.split('.').any { label ->
                label.isEmpty() || label.length > 63 || label.first() == '-' || label.last() == '-' || label.any { it !in 'a'..'z' && it !in '0'..'9' && it != '-' }
            }) throw invalid()
        if ('.' in dns && dns.split('.').last().first() !in 'a'..'z' && !ipv4(host)) throw invalid()
        if (dns.all { it in '0'..'9' || it == '.' } && !ipv4(host)) throw invalid()
    }
    return raw
}

private fun ipv4(raw: String): Boolean {
    val parts = raw.split('.')
    return parts.size == 4 && parts.all { it.isNotEmpty() && it.all { digit -> digit in '0'..'9' } &&
        (it == "0" || it[0] != '0') && (it.toIntOrNull() ?: 256) <= 255 }
}
