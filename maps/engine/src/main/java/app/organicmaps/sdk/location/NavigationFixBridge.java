package app.organicmaps.sdk.location;

/** Abastevo bridge to the package-private native location feed.
 * Forwards primitive fixes without exposing SDK types to the app. */
public final class NavigationFixBridge {
  private NavigationFixBridge() {}

  public static void forward(
      long time,
      double lat,
      double lon,
      float accuracy,
      double altitude,
      float speed,
      float bearing) {
    LocationState.nativeLocationUpdated(time, lat, lon, accuracy, altitude, speed, bearing);
  }
}
