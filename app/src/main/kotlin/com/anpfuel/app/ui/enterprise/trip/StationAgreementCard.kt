package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import com.anpfuel.app.R
import com.anpfuel.application.enterprise.agreement.FieldVoucherNotice

/**
 * EMP-AGREE-004 / EMP-STN-003: Driver-safe fueling authorization summary
 * inside the trip card showing the nearest usable authorization with its
 * limits and expiry, or the recorded refusal state. The one-time voucher
 * code is shown only through the separate secure redemption flow, never
 * inside this summary card.
 */
@Composable
internal fun StationAgreementCard(
    notice: FieldVoucherNotice?,
    nowEpochSeconds: Long,
    modifier: Modifier = Modifier,
) {
    TripCard(modifier.testTag("enterprise-station-agreement-card")) {
        Text(
            text = stringResource(R.string.enterprise_agreement_card_title),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() },
        )
        when {
            notice == null -> {
                Text(
                    text = stringResource(R.string.enterprise_agreement_voucher_locked),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            !notice.eligibleAt(nowEpochSeconds) -> {
                val expired = notice.derivedStateAt(nowEpochSeconds) == "EXPIRED"
                Text(
                    text = stringResource(
                        if (expired) R.string.enterprise_agreement_voucher_expired
                        else R.string.enterprise_agreement_voucher_locked,
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            else -> {
                Text(
                    text = stringResource(
                        R.string.enterprise_agreement_voucher_limits,
                        notice.maxQuantityMilli,
                        notice.maxCentavos,
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    text = stringResource(
                        R.string.enterprise_agreement_voucher_valid_until,
                        com.anpfuel.application.enterprise.agreement.voucherValidUntilText(notice),
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        }
        Text(
            text = stringResource(R.string.enterprise_agreement_announced_source),
            style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}
