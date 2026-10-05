package com.anpfuel.app.ui.stationprofile;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;

/** Standalone test APK process: uses Java/Android only, no target-app runtime. */
public final class SyntheticDocumentProvider extends ContentProvider {
    @Override public boolean onCreate() { return true; }
    @Override public String getType(Uri uri) { return "application/pdf"; }
    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] args, String sort) {
        MatrixCursor cursor = new MatrixCursor(new String[] {"_display_name", "_size"});
        cursor.addRow(new Object[] {"synthetic.pdf", -1});
        return cursor;
    }
    @Override public Uri insert(Uri uri, ContentValues values) { return null; }
    @Override public int update(Uri uri, ContentValues values, String selection, String[] args) { return 0; }
    @Override public int delete(Uri uri, String selection, String[] args) { return 0; }
    @Override public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if ("expired".equals(uri.getLastPathSegment())) throw new FileNotFoundException("Access expired");
        final ParcelFileDescriptor[] pipes;
        try { pipes = ParcelFileDescriptor.createPipe(); }
        catch (IOException error) { throw new FileNotFoundException("Test pipe unavailable"); }
        new Thread(() -> {
            try (ParcelFileDescriptor.AutoCloseOutputStream stream = new ParcelFileDescriptor.AutoCloseOutputStream(pipes[1])) {
                stream.write("%PDF-1.7\n".getBytes(StandardCharsets.UTF_8));
                byte[] block = new byte[8192];
                Arrays.fill(block, (byte) 65);
                int blocks = "oversize".equals(uri.getLastPathSegment()) ? 641 : 639;
                for (int index = 0; index < blocks; index++) stream.write(block);
            } catch (IOException expected) { /* Reader closes a deliberately rejected pipe. */ }
        }, "synthetic-document").start();
        return pipes[0];
    }
}
