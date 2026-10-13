package com.anpfuel.app.games

import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import com.anpfuel.domain.games.GameId

/** Preview shares the actual scene palette and silhouettes; no loop, input or stored score. */
@Composable
internal fun GameArtwork(game: GameId, modifier: Modifier = Modifier) {
    val scene = remember(game) { ArcadeScene() }
    Canvas(modifier) {
        drawIntoCanvas { canvas ->
            canvas.nativeCanvas.save()
            canvas.nativeCanvas.clipRect(0f, 0f, size.width, size.height)
            val scale = maxOf(size.width / 1000f, size.height / 520f)
            canvas.nativeCanvas.translate((size.width - 1000f * scale) / 2, (size.height - 520f * scale) / 2)
            canvas.nativeCanvas.scale(scale, scale)
            scene.preview(canvas.nativeCanvas, game)
            canvas.nativeCanvas.restore()
        }
    }
}
