package com.anpfuel.app.capture

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.unmockkStatic
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class CameraPermissionHandlerTest {

    private val context = mockk<Context>(relaxed = true)
    private lateinit var handler: CameraPermissionHandler

    @BeforeEach
    fun setUp() {
        mockkStatic(ContextCompat::class)
        handler = CameraPermissionHandler(context)
    }

    @AfterEach
    fun tearDown() {
        unmockkStatic(ContextCompat::class)
    }

    @Test
    fun hasCameraPermissionWhenGranted() {
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA)
        } returns PackageManager.PERMISSION_GRANTED

        assertTrue(handler.hasCameraPermission())
    }

    @Test
    fun lacksCameraPermissionWhenDenied() {
        every {
            ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA)
        } returns PackageManager.PERMISSION_DENIED

        assertFalse(handler.hasCameraPermission())
    }
}
