package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.application.portable.PhotoClock
import com.anpfuel.application.portable.PhotoDecoder
import com.anpfuel.application.portable.PhotoEncoder
import com.anpfuel.application.portable.PhotoFlow
import com.anpfuel.application.portable.PhotoIdSource
import com.anpfuel.application.portable.PhotoPorts
import com.anpfuel.data.local.media.AndroidPhotoCache
import com.anpfuel.data.local.media.AndroidPhotoCodec
import com.anpfuel.data.local.media.AndroidPhotoKeystoreKeys
import com.anpfuel.data.local.media.PhotoKeyProvider
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import java.util.UUID
import javax.inject.Singleton

/**
 * Lightweight-photo Hilt bindings (P15-T02B).
 *
 * [PhotoFlow] stays platform-free; Android owns the codec
 * (BitmapFactory sample-decode), sealed transient files (Keystore)
 * plus time and ids here. The transient directory is private
 * app cache (`anpfuel_photos`); nothing is ever written to shared
 * media, backups or logs. Encode/decode work stays off the UI
 * thread at the call site, one image at a time (M02).
 */
@Module
@InstallIn(SingletonComponent::class)
object MediaModule {

    @Provides
    @Singleton
    fun providePhotoKeys(): PhotoKeyProvider = AndroidPhotoKeystoreKeys()

    @Provides
    @Singleton
    fun providePhotoCacheDir(@ApplicationContext context: Context): File {
        return File(context.cacheDir, AndroidPhotoCache.SUBDIR)
    }

    @Provides
    @Singleton
    fun providePhotoCache(dir: File, keys: PhotoKeyProvider): PhotoCache {
        return AndroidPhotoCache(dir, keys)
    }

    @Provides
    @Singleton
    fun providePhotoCodec(): AndroidPhotoCodec = AndroidPhotoCodec()

    @Provides
    @Singleton
    fun providePhotoDecoder(codec: AndroidPhotoCodec): PhotoDecoder = codec

    @Provides
    @Singleton
    fun providePhotoEncoder(codec: AndroidPhotoCodec): PhotoEncoder = codec

    @Provides
    @Singleton
    fun providePhotoFlow(
        cache: PhotoCache,
        decoder: PhotoDecoder,
        encoder: PhotoEncoder,
    ): PhotoFlow {
        val ports = PhotoPorts(
            decoder = decoder,
            encoder = encoder,
            cache = cache,
            clock = PhotoClock { System.currentTimeMillis() },
            ids = PhotoIdSource { UUID.randomUUID().toString() },
        )
        return PhotoFlow(ports)
    }
}
