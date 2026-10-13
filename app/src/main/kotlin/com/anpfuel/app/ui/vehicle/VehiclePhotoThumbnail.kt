package com.anpfuel.app.ui.vehicle

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DirectionsCar
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.data.local.media.BoundedPhotoBitmap
import com.anpfuel.data.local.media.VehiclePhotoStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Square vehicle photo thumbnail, nearly filling its slot with the card
 * shape. Local-only file; a directions icon stands in when absent.
 */
@Composable
fun VehiclePhotoThumbnail(
    photoPath: String?,
    size: Dp,
    modifier: Modifier = Modifier,
    contentDescription: String? = null,
) {
    val context = LocalContext.current
    var bitmap by remember(photoPath) { mutableStateOf<Bitmap?>(null) }
    LaunchedEffect(photoPath) {
        bitmap = withContext(Dispatchers.IO) {
            runCatching {
                val bytes = photoPath
                    ?.let { VehiclePhotoStore(context.applicationContext).readPhotoBytes(it) }
                    ?: return@runCatching null
                BoundedPhotoBitmap.decode(bytes, MAX_SIDE_PX)
            }.getOrNull()
        }
    }
    Box(
        modifier = modifier
            .size(size)
            .clip(RoundedCornerShape(CORNER_DP.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant),
        contentAlignment = Alignment.Center,
    ) {
        val current = bitmap
        if (current != null) {
            Image(
                bitmap = current.asImageBitmap(),
                contentDescription = contentDescription
                    ?: stringResource(R.string.vehicle_photo_content),
                contentScale = ContentScale.Crop,
                modifier = Modifier.matchParentSize(),
            )
        } else {
            Icon(
                imageVector = Icons.Default.DirectionsCar,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

private const val CORNER_DP = 12
private const val MAX_SIDE_PX = 256
