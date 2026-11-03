use serde_json::json;
use station_prep::ev_geography::StateIndex;

fn square(x: f64, y: f64, size: f64) -> serde_json::Value {
    json!([
        [x, y],
        [x + size, y],
        [x + size, y + size],
        [x, y + size],
        [x, y]
    ])
}

#[test]
fn state_index_preserves_holes_exclaves_and_ambiguous_boundaries() {
    let outer = square(-50.0, -20.0, 2.0);
    let hole = square(-49.5, -19.5, 0.5);
    let exclave = square(-45.0, -20.0, 0.5);
    let raw=serde_json::to_vec(&json!({"type":"FeatureCollection","features":[
  {"type":"Feature","properties":{"codarea":"3500000","fixture":"[P25-TEST] synthetic SP shape"},"geometry":{"type":"MultiPolygon","coordinates":[[outer,hole],[exclave]]}},
  {"type":"Feature","properties":{"codarea":"3300000","fixture":"[P25-TEST] synthetic RJ overlap"},"geometry":{"type":"Polygon","coordinates":[square(-48.5,-19.0,1.0)]}}
 ]})).unwrap();
    let index = StateIndex::from_bytes(&raw).unwrap();
    assert_eq!(index.classify(-19.8, -49.8).uf.as_deref(), Some("SP"));
    assert_eq!(index.classify(-19.3, -49.3).reason, "no_boundary_match");
    assert_eq!(index.classify(-19.8, -44.8).uf.as_deref(), Some("SP"));
    assert_eq!(index.classify(-18.8, -48.2).reason, "ambiguous_uf");
    assert_eq!(index.classify(-20.0, -50.0).reason, "boundary_ambiguous");
    assert_eq!(index.classify(f64::NAN, -49.8).reason, "invalid_point");
    assert_eq!(index.source_sha256.len(), 64);
}

#[test]
fn state_index_refuses_unknown_codes_and_unclosed_or_nonfinite_rings() {
    for fault in ["code", "ring", "coordinate"] {
        let mut feature = json!({"type":"Feature","properties":{"codarea":"3500000"},"geometry":{"type":"Polygon","coordinates":[square(-50.0,-20.0,1.0)]}});
        if fault == "code" {
            feature["properties"]["codarea"] = json!("9900000");
        }
        if fault == "ring" {
            feature["geometry"]["coordinates"][0]
                .as_array_mut()
                .unwrap()
                .pop();
        }
        if fault == "coordinate" {
            feature["geometry"]["coordinates"][0][0][0] = json!(500.0);
        }
        let raw =
            serde_json::to_vec(&json!({"type":"FeatureCollection","features":[feature]})).unwrap();
        assert!(StateIndex::from_bytes(&raw).is_err(), "{fault}");
    }
}
