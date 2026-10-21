//! EE-03 byte/identity goldens, separate from future source preparation.
use serde_json::Value;
use station_prep::ev_contract::{canonical_bytes, source_key};

#[test]
fn frozen_utf8_bytes_and_source_identity() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../contracts/testdata/ev/prepared-v1/byte-goldens.json"
    ))
    .unwrap();
    for case in fixture["canonical"].as_array().unwrap() {
        assert_eq!(
            canonical_bytes(&case["value"]).unwrap(),
            case["expected_utf8"].as_str().unwrap().as_bytes()
        );
    }
    for case in fixture["identities"].as_array().unwrap() {
        assert_eq!(
            source_key(
                case["namespace"].as_str().unwrap(),
                case["kind"].as_str().unwrap(),
                case["external_id"].as_str().unwrap()
            )
            .unwrap(),
            case["sha256"].as_str().unwrap()
        );
    }
    assert!(canonical_bytes(&serde_json::json!({"a": 1.5})).is_err());
    assert!(canonical_bytes(&serde_json::json!({"a": 9007199254740992_u64})).is_err());
    assert!(source_key("ocm", "PLACE", "").is_err());
    assert_ne!(
        source_key("ab", "PLACE", "c").unwrap(),
        source_key("a", "PLACE", "bc").unwrap()
    );
    assert_ne!(
        source_key("ocm", "PLACE", "42").unwrap(),
        source_key("ocm", "CONNECTION_GROUP", "42").unwrap()
    );
}

#[test]
fn bounded_canonical_and_identity_failures() {
    for value in [
        serde_json::json!({"á": "ASCII keys only"}),
        serde_json::json!("x".repeat(65_537)),
        serde_json::json!(-9_007_199_254_740_992_i64),
    ] {
        assert!(canonical_bytes(&value).is_err());
    }
    let mut deep = serde_json::Value::Null;
    for _ in 0..40 {
        deep = serde_json::json!([deep]);
    }
    assert!(canonical_bytes(&deep).is_err());
    let oversized = serde_json::json!(vec!["x".repeat(65_536); 17]);
    assert!(canonical_bytes(&oversized).is_err());
    for bad in ["", "\u{85}", "line\nbreak", &"x".repeat(257)] {
        assert!(source_key("ocm", "PLACE", bad).is_err());
    }
    assert!(source_key("OCM", "PLACE", "1").is_err());
    assert!(source_key("ocm", "UNKNOWN", "1").is_err());
}
