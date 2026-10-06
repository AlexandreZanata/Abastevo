package com.anpfuel.app.ui.home

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.AddCircle
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

internal object HomeHeroColors {
    val Navy = Color(0xFF061E2C)
    val Mint = Color(0xFF63E7A4)
    val OnMint = Color(0xFF063B2B)
}

@Composable
internal fun HomeCommunityHero(onContribute: () -> Unit) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(24.dp))
            .background(HomeHeroColors.Navy),
    ) {
        Image(
            painter = painterResource(R.drawable.home_community_hero),
            contentDescription = null,
            contentScale = ContentScale.Crop,
            alignment = Alignment.CenterEnd,
            modifier = Modifier.matchParentSize(),
        )
        Box(
            modifier = Modifier.matchParentSize().background(
                Brush.horizontalGradient(
                    0f to HomeHeroColors.Navy.copy(alpha = 0.96f),
                    0.65f to HomeHeroColors.Navy.copy(alpha = 0.88f),
                    1f to HomeHeroColors.Navy.copy(alpha = 0.16f),
                ),
            ),
        )
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Column(modifier = Modifier.fillMaxWidth(0.78f)) {
                Text(
                    text = stringResource(R.string.home_hero_title),
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                    color = Color.White,
                )
                Text(
                    text = stringResource(R.string.home_hero_title_accent),
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                    color = HomeHeroColors.Mint,
                )
                Text(
                    text = stringResource(R.string.home_hero_description),
                    modifier = Modifier.padding(top = 4.dp),
                    style = MaterialTheme.typography.bodySmall,
                    color = Color.White,
                )
            }
            Button(
                onClick = onContribute,
                shape = RoundedCornerShape(14.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = HomeHeroColors.Mint,
                    contentColor = HomeHeroColors.OnMint,
                ),
            ) {
                Icon(Icons.Default.AddCircle, contentDescription = null, modifier = Modifier.size(20.dp))
                Text(
                    text = stringResource(R.string.home_hero_contribute),
                    modifier = Modifier.weight(1f, fill = false).padding(horizontal = 8.dp),
                    style = MaterialTheme.typography.labelLarge,
                )
                Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, modifier = Modifier.size(18.dp))
            }
        }
    }
}

private data class HomeBenefit(val icon: Int, val title: Int, val tint: Color)

@Composable
internal fun HomeBenefits() {
    val benefits = listOf(
        HomeBenefit(R.drawable.ic_home_savings, R.string.home_benefit_savings, Color(0xFF209437)),
        HomeBenefit(R.drawable.ic_home_community, R.string.home_benefit_community, Color(0xFF1478DD)),
        HomeBenefit(R.drawable.ic_home_trust, R.string.home_benefit_trust, Color(0xFFF57C00)),
    )
    Card(
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Row(modifier = Modifier.fillMaxWidth().height(IntrinsicSize.Min).padding(vertical = 16.dp)) {
            benefits.forEachIndexed { index, benefit ->
                Column(
                    modifier = Modifier.weight(1f).padding(horizontal = 8.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    BenefitIcon(benefit)
                    Text(
                        text = stringResource(benefit.title),
                        modifier = Modifier.fillMaxWidth(),
                        style = MaterialTheme.typography.labelLarge,
                        fontWeight = FontWeight.Bold,
                        textAlign = TextAlign.Center,
                    )
                }
                if (index < benefits.lastIndex) {
                    VerticalDivider(
                        modifier = Modifier.fillMaxHeight(),
                        color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f),
                    )
                }
            }
        }
    }
}

@Composable
private fun BenefitIcon(benefit: HomeBenefit) {
    Box(
        contentAlignment = Alignment.Center,
        modifier = Modifier.size(44.dp).background(benefit.tint.copy(alpha = 0.12f), CircleShape),
    ) {
        Icon(
            painter = painterResource(benefit.icon),
            contentDescription = null,
            tint = Color.Unspecified,
            modifier = Modifier.size(28.dp),
        )
    }
}
