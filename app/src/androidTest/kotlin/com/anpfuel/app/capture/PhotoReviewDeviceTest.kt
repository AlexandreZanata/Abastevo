package com.anpfuel.app.capture

import android.content.Context
import android.graphics.Bitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.R
import com.anpfuel.data.local.media.AndroidPhotoCodec
import com.anpfuel.data.local.media.BoundedPhotoBitmap
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.domain.portable.PortablePhoto
import java.io.ByteArrayOutputStream
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Private original corpus is pushed by owner-authorized ADB, never committed or uploaded. */
@RunWith(AndroidJUnit4::class)
class PhotoReviewDeviceTest {
    @get:Rule val compose = createAndroidComposeRule<PhotoEvaluationActivity>()

    @Test fun realPhotoDetectedSubsetEditRemoveAndLocalConfirmation() {
        compose.runOnUiThread { compose.activity.loadEvaluationImage("ocr-0001.jpeg") }
        compose.waitUntil(20_000) { !compose.activity.processing && compose.activity.visibleRowCount == 4 }
        compose.onAllNodes(hasSetTextAction()).assertCountEquals(4)
        compose.onAllNodes(hasSetTextAction())[0].performTextReplacement("6,70")
        val removeLabel = compose.activity.getString(R.string.capture_remove_fuel)
        compose.onAllNodesWithContentDescription(removeLabel)[1].performScrollTo().performClick()
        compose.onAllNodes(hasSetTextAction()).assertCountEquals(3)
        compose.onAllNodes(hasSetTextAction())[1].performTextClearance()
        val send = compose.activity.getString(R.string.capture_send_prices, 2)
        compose.onNodeWithText(send).performScrollTo().performClick()
        compose.runOnIdle {
            assertTrue(compose.activity.queuedLocally)
            assertEquals(2, compose.activity.reviewedCount)
        }
    }

    @Test fun controlledOutsideStationShowsInstructionsWithoutOpeningCamera() {
        compose.onNodeWithText("Fora: 151 m").performClick()
        compose.onNodeWithText("Contribuir com foto (teste)").performClick()
        compose.onNodeWithText("Fique a até 150 metros do posto. Câmera não aberta.").assertIsDisplayed()
        compose.runOnIdle { assertEquals(0, compose.activity.visibleRowCount) }
    }

    @Test fun privateAgeBoundsAndUprightWirePixelsStayInsideFrozenBudgets() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val files = PrivateCaptureFiles(context)
        val now = System.currentTimeMillis()
        val image = Bitmap.createBitmap(2400, 2400, Bitmap.Config.ARGB_8888)
        val bytes = ByteArrayOutputStream().use { output -> image.compress(Bitmap.CompressFormat.JPEG, 95, output); output.toByteArray() }
        image.recycle()
        val uri = files.write(bytes, now - 60000)
        try {
            assertNotNull(files.readReview(uri))
            // Expire only this owned synthetic URI; never sweep other drafts using a future clock.
            assertNull(files.read(uri, now - 86_400_000))
            assertNull(files.readReview(uri))
        } finally { files.delete(uri) }
        val codec = AndroidPhotoCodec()
        val dims = codec.probeDims(bytes)!!
        val wire = codec.encode(bytes, PhotoFlow.EncodeRequest(PortablePhoto.sampleSizeForBounds(dims.width, dims.height), 1))!!
        val decoded = BoundedPhotoBitmap.decode(wire)
        try {
            assertTrue(decoded.width <= 1600 && decoded.height <= 1600)
            assertTrue(decoded.width.toLong() * decoded.height <= 2_000_000)
        } finally { decoded.recycle() }
    }
    @Test fun zoomCropAndRecreationPreserveHumanEdits() {
        compose.runOnUiThread { compose.activity.loadEvaluationImage("ocr-0001.jpeg") }
        compose.waitUntil(20_000) { !compose.activity.processing && compose.activity.visibleRowCount == 4 }
        compose.onAllNodes(hasSetTextAction())[0].performTextReplacement("4,99")
        val photo = compose.activity.getString(R.string.capture_photo_content)
        compose.onNodeWithContentDescription(photo).performScrollTo().performTouchInput {
            pinch(center - androidx.compose.ui.geometry.Offset(40f, 0f), center + androidx.compose.ui.geometry.Offset(40f, 0f),
                center - androidx.compose.ui.geometry.Offset(85f, 0f), center + androidx.compose.ui.geometry.Offset(85f, 0f), 600)
        }
        val crop = compose.activity.getString(R.string.capture_crop)
        compose.onNodeWithText(crop).performScrollTo().performClick()
        compose.waitUntil(20_000) { !compose.activity.processing }
        compose.onNodeWithText("4,99").assertExists()
        compose.activityRule.scenario.recreate()
        compose.waitForIdle()
        compose.onNodeWithText("4,99").assertExists()
        // The original edited fuel survives even when the crop has fewer machine suggestions.
        compose.onAllNodes(hasSetTextAction()).fetchSemanticsNodes().let { assertTrue(it.isNotEmpty()) }
    }

}
