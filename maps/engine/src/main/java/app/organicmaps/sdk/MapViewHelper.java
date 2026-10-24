package app.organicmaps.sdk;

import androidx.annotation.NonNull;
import app.organicmaps.sdk.location.LocationHelper;

/** Abastevo-owned bridge to the package-private MapView/Map relationship. */
public final class MapViewHelper {
  private MapViewHelper() {}

  public static void setLocationHelper(@NonNull MapView view, @NonNull LocationHelper helper) {
    view.getMap().setLocationHelper(helper);
  }
}
