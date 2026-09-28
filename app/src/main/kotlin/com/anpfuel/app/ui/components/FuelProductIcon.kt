package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.tooling.preview.PreviewParameter
import androidx.compose.ui.tooling.preview.PreviewParameterProvider
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.anpfuel.app.mapper.FuelProductDrawable
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.FuelProduct

@Composable
fun FuelProductIcon(
    product: FuelProduct,
    modifier: Modifier = Modifier,
    size: Dp = 24.dp,
    contentDescription: String? = stringResource(FuelProductI18n.toStringRes(product)),
) {
    Icon(
        painter = painterResource(FuelProductDrawable.toDrawableRes(product)),
        contentDescription = contentDescription,
        tint = Color.Unspecified,
        modifier = modifier
            .size(size)
            .then(
                if (contentDescription != null) {
                    Modifier.semantics { this.contentDescription = contentDescription }
                } else {
                    Modifier.clearAndSetSemantics {}
                },
            ),
    )
}

@Composable
fun FuelProductLabel(
    product: FuelProduct,
    modifier: Modifier = Modifier,
    iconSize: Dp = 20.dp,
) {
    val label = stringResource(FuelProductI18n.toStringRes(product))

    Row(
        modifier = modifier.semantics(mergeDescendants = true) {
            contentDescription = label
        },
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FuelProductIcon(
            product = product,
            size = iconSize,
            contentDescription = null,
        )
        Text(
            text = label,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurface,
        )
    }
}

private class FuelProductPreviewProvider : PreviewParameterProvider<FuelProduct> {
    override val values: Sequence<FuelProduct> = FuelProduct.entries.asSequence()
}

@Preview(showBackground = true, name = "Light")
@Composable
private fun FuelProductIconLightPreview(
    @PreviewParameter(FuelProductPreviewProvider::class) product: FuelProduct,
) {
    AnpFuelTheme(darkTheme = false, dynamicColor = false) {
        FuelProductIcon(product = product, modifier = Modifier.size(32.dp))
    }
}

@Preview(showBackground = true, name = "Dark")
@Composable
private fun FuelProductIconDarkPreview(
    @PreviewParameter(FuelProductPreviewProvider::class) product: FuelProduct,
) {
    AnpFuelTheme(darkTheme = true, dynamicColor = false) {
        FuelProductIcon(product = product, modifier = Modifier.size(32.dp))
    }
}

@Preview(showBackground = true)
@Composable
private fun FuelProductLabelPreview() {
    AnpFuelTheme(dynamicColor = false) {
        FuelProductLabel(product = FuelProduct.GASOLINE_REGULAR)
    }
}
