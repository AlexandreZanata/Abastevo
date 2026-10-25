package app.organicmaps.sdk;

import android.content.Context;
import android.content.ContextWrapper;
import androidx.annotation.NonNull;
import androidx.lifecycle.DefaultLifecycleObserver;
import androidx.lifecycle.LifecycleOwner;
import app.organicmaps.sdk.location.LocationHelper;

/** Abastevo bridge to the package-private SDK map lifecycle. */
public final class MapViewHelper {
  private MapViewHelper() {}

  public static MapView create(@NonNull Context context, @NonNull LocationHelper helper) {
    Context host = context;
    while (!(host instanceof LifecycleOwner) && host instanceof ContextWrapper) {
      Context base = ((ContextWrapper) host).getBaseContext();
      if (base == host) break;
      host = base;
    }
    if (!(host instanceof LifecycleOwner)) {
      throw new IllegalArgumentException("Map surface requires an AndroidX lifecycle host");
    }
    MapView view = new LifecycleMapView(context, (LifecycleOwner) host);
    view.getMap().setLocationHelper(helper);
    return view;
  }

  private static final class LifecycleMapView extends MapView {
    private final LifecycleOwner owner;
    private final DefaultLifecycleObserver observer = new DefaultLifecycleObserver() {
      @Override public void onStart(@NonNull LifecycleOwner source) { getMap().onStart(); }
      @Override public void onResume(@NonNull LifecycleOwner source) { getMap().onResume(); }
      @Override public void onPause(@NonNull LifecycleOwner source) { getMap().onPause(); }
      @Override public void onStop(@NonNull LifecycleOwner source) { getMap().onStop(); }
    };

    LifecycleMapView(Context context, LifecycleOwner owner) {
      super(context);
      this.owner = owner;
      getMap().onCreate(false);
    }

    @Override protected void onAttachedToWindow() {
      super.onAttachedToWindow();
      owner.getLifecycle().addObserver(observer);
    }

    @Override protected void onDetachedFromWindow() {
      // SurfaceView releases buffers inside super; stop native drawing first.
      getMap().onPause();
      getMap().onStop();
      owner.getLifecycle().removeObserver(observer);
      super.onDetachedFromWindow();
    }
  }
}
