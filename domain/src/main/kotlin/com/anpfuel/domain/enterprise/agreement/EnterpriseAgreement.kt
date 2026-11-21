package com.anpfuel.domain.enterprise.agreement

/**
 * EMP-AGREE-001..006 (BR-AGREE-001/002): the pure Phase F7 station agreement,
 * negotiated price, bounded fueling authorization and redemption rules shared
 * with the backend golden vectors. The one-time voucher code never enters
 * this domain; only its server-stored digest exists there.
 */
object AgreementRules {
    const val METHOD_VERSION = "AGREEMENT_RULES_V1"

    val PRODUCTS = setOf(
        "ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED",
        "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13",
    )
    val UNITS = setOf("L", "M3", "KG_13")

    fun validProduct(product: String, unit: String): Boolean =
        product in PRODUCTS && unit in UNITS

    /**
     * Settlement total in centavos (BR-MONEY-005): milli-units times milli-BRL
     * per unit divided by 10 000, HALF_UP.
     */
    fun totalCentavos(quantityMilli: Long, priceMilliBrl: Long): Long {
        if (quantityMilli < 0L || priceMilliBrl < 0L) return 0L
        val num = quantityMilli * priceMilliBrl + 5000L
        return num / 10000L
    }
}

enum class AgreementError {
    AGREEMENT_TERMS_REQUIRED,
    AGREEMENT_PRODUCT_INVALID,
    AGREEMENT_PRICE_INVALID,
    AGREEMENT_CONDITIONS_INVALID,
    AGREEMENT_TERM_DUPLICATE,
    AGREEMENT_WINDOW_INVALID,
    AGREEMENT_TRANSITION_REFUSED,
    AUTHORIZATION_EXPIRY_INVALID,
    AUTHORIZATION_LIMITS_INVALID,
    AUTHORIZATION_PRODUCT_INVALID,
    AUTHORIZATION_TRANSITION_REFUSED,
    AUTHORIZATION_EXPIRED,
    AUTHORIZATION_PRODUCT_MISMATCH,
    AUTHORIZATION_QUANTITY_EXCEEDED,
    AUTHORIZATION_TOTAL_EXCEEDED,
    AUTHORIZATION_LOCKED,
}

/** Stored agreement states; EXPIRED derives at read time. */
enum class AgreementState {
    PROPOSED, ACTIVE, SUSPENDED, ENDED, DECLINED, EXPIRED;

    companion object {
        fun derived(state: String, nowEpochSeconds: Long, fromEpochSeconds: Long, untilEpochSeconds: Long): String {
            if (state == ACTIVE.name && untilEpochSeconds > 0L && nowEpochSeconds > untilEpochSeconds) {
                return EXPIRED.name
            }
            return state
        }
    }
}

data class AgreementTerm(
    val product: String,
    val unit: String,
    val priceMilliBrl: Long,
    val conditions: String = "",
) {
    init {
        require(AgreementRules.validProduct(product, unit)) { AgreementError.AGREEMENT_PRODUCT_INVALID.name }
        require(priceMilliBrl in 1..1_000_000L) { AgreementError.AGREEMENT_PRICE_INVALID.name }
        require(conditions.length <= 120) { AgreementError.AGREEMENT_CONDITIONS_INVALID.name }
    }
}

data class AgreementHead(
    val state: AgreementState,
    val revision: Long = 1L,
    val fromEpochSeconds: Long = 0L,
    val untilEpochSeconds: Long = 0L,
    val terms: List<AgreementTerm> = emptyList(),
) {
    fun usableAt(nowEpochSeconds: Long): Boolean =
        state == AgreementState.ACTIVE && nowEpochSeconds >= fromEpochSeconds && nowEpochSeconds <= untilEpochSeconds

    fun accept(): AgreementHead = transition(AgreementState.ACTIVE)
    fun decline(): AgreementHead = transition(AgreementState.DECLINED)
    fun suspend(): AgreementHead = transition(AgreementState.SUSPENDED)
    fun end(): AgreementHead = transition(AgreementState.ENDED)

    private fun transition(to: AgreementState): AgreementHead {
        val allowed = when (state) {
            AgreementState.PROPOSED -> to == AgreementState.ACTIVE || to == AgreementState.DECLINED || to == AgreementState.ENDED
            AgreementState.ACTIVE -> to == AgreementState.SUSPENDED || to == AgreementState.ENDED
            AgreementState.SUSPENDED -> to == AgreementState.ENDED
            else -> false
        }
        require(allowed) { AgreementError.AGREEMENT_TRANSITION_REFUSED.name }
        return copy(state = to, revision = revision + 1L)
    }

    companion object {
        /** UC-AGREE-01: the proposal carries at least one exact term inside its window. */
        fun propose(
            fromEpochSeconds: Long,
            untilEpochSeconds: Long,
            terms: List<AgreementTerm>,
        ): AgreementHead {
            require(terms.isNotEmpty()) { AgreementError.AGREEMENT_TERMS_REQUIRED.name }
            require(untilEpochSeconds > fromEpochSeconds) { AgreementError.AGREEMENT_WINDOW_INVALID.name }
            val keys = terms.map { it.product + "/" + it.unit }
            require(keys.size == keys.distinct().size) { AgreementError.AGREEMENT_TERM_DUPLICATE.name }
            return AgreementHead(
                state = AgreementState.PROPOSED,
                revision = 1L,
                fromEpochSeconds = fromEpochSeconds,
                untilEpochSeconds = untilEpochSeconds,
                terms = terms,
            )
        }
    }
}

/** Stored authorization states; EXPIRED derives at read time. */
enum class VoucherState { ISSUED, REDEEMED, CANCELLED, LOCKED, EXPIRED }

data class Voucher(
    val state: VoucherState,
    val product: String,
    val unit: String,
    val maxQuantityMilli: Long,
    val maxCentavos: Long,
    val validUntilEpochSeconds: Long,
    val failedAttempts: Int = 0,
) {
    init {
        require(AgreementRules.validProduct(product, unit)) { "authorization product invalid" }
        require(maxQuantityMilli >= 1L && maxCentavos >= 1L) { "authorization limits invalid" }
        require(failedAttempts in 0..5) { AgreementError.AUTHORIZATION_LOCKED.name }
    }

    fun derivedStateAt(nowEpochSeconds: Long): VoucherState =
        if (state == VoucherState.ISSUED && validUntilEpochSeconds > 0L && nowEpochSeconds > validUntilEpochSeconds) {
            VoucherState.EXPIRED
        } else {
            state
        }

    fun eligibleAt(nowEpochSeconds: Long): Boolean = derivedStateAt(nowEpochSeconds) == VoucherState.ISSUED

    /** One bounded invalid try: the fifth locks the voucher (BR-TEN-005). */
    fun failedAttempt(): Voucher {
        require(state == VoucherState.ISSUED) { AgreementError.AUTHORIZATION_TRANSITION_REFUSED.name }
        val next = failedAttempts + 1
        return copy(failedAttempts = next, state = if (next >= 5) VoucherState.LOCKED else VoucherState.ISSUED)
    }

    /**
     * EMP-AGREE-005: one consume under the recorded limits. Matching retries
     * are answered by the server's stored receipt; this pure check decides a
     * single consume and never mutates the voucher.
     */
    fun consume(
        product: String,
        unit: String,
        quantityMilli: Long,
        totalCentavos: Long,
        nowEpochSeconds: Long,
    ) {
        when (derivedStateAt(nowEpochSeconds)) {
            VoucherState.LOCKED -> throw IllegalStateException(AgreementError.AUTHORIZATION_LOCKED.name)
            VoucherState.EXPIRED -> throw IllegalStateException(AgreementError.AUTHORIZATION_EXPIRED.name)
            VoucherState.CANCELLED, VoucherState.REDEEMED -> throw IllegalStateException(AgreementError.AUTHORIZATION_TRANSITION_REFUSED.name)
            VoucherState.ISSUED -> Unit
        }
        if (product != this.product || unit != this.unit) {
            throw IllegalStateException(AgreementError.AUTHORIZATION_PRODUCT_MISMATCH.name)
        }
        if (quantityMilli < 1L || quantityMilli > maxQuantityMilli) {
            throw IllegalStateException(AgreementError.AUTHORIZATION_QUANTITY_EXCEEDED.name)
        }
        if (totalCentavos < 1L || totalCentavos > maxCentavos) {
            throw IllegalStateException(AgreementError.AUTHORIZATION_TOTAL_EXCEEDED.name)
        }
    }

    companion object {
        /** EMP-AGREE-004: issuance validates the exact bounded fields. */
        fun issue(
            product: String,
            unit: String,
            maxQuantityMilli: Long,
            maxCentavos: Long,
            validUntilEpochSeconds: Long,
            nowEpochSeconds: Long,
        ): Voucher {
            if (!AgreementRules.validProduct(product, unit)) {
                throw IllegalStateException(AgreementError.AUTHORIZATION_PRODUCT_INVALID.name)
            }
            if (maxQuantityMilli < 1L || maxCentavos < 1L) {
                throw IllegalStateException(AgreementError.AUTHORIZATION_LIMITS_INVALID.name)
            }
            if (validUntilEpochSeconds <= nowEpochSeconds) {
                throw IllegalStateException(AgreementError.AUTHORIZATION_EXPIRY_INVALID.name)
            }
            return Voucher(
                state = VoucherState.ISSUED,
                product = product,
                unit = unit,
                maxQuantityMilli = maxQuantityMilli,
                maxCentavos = maxCentavos,
                validUntilEpochSeconds = validUntilEpochSeconds,
            )
        }
    }
}
