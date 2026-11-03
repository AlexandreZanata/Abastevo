use station_prep::station_addresses::{
    select_address, AddressCandidate, AddressSource, MatchProof, Provenance, ReuseDecision,
    ReuseStatus, StationAddressInput,
};
use std::collections::BTreeMap;
fn address(street: &str, number: &str) -> BTreeMap<String, String> {
    [
        ("street", street),
        ("number", number),
        ("district", "[P25-TEST] District"),
    ]
    .into_iter()
    .map(|(k, v)| (k.into(), v.into()))
    .collect()
}
fn station() -> StationAddressInput {
    StationAddressInput {
        cnpj: "91000000000066".into(),
        municipality_ibge: "3550308".into(),
        municipality: "[P25-TEST] City".into(),
        uf: "SP".into(),
        official_address: BTreeMap::new(),
    }
}
fn candidate(source: AddressSource) -> AddressCandidate {
    let proof = match source {
        AddressSource::Rfb => MatchProof::Rfb {
            cnpj: station().cnpj,
            active: true,
        },
        AddressSource::Osm => MatchProof::Osm {
            cnpj: station().cnpj,
            object_type: "node".into(),
            object_id: 90001,
            link_reference: "[P25-TEST] reviewed station link".into(),
        },
        AddressSource::Cnefe => MatchProof::Cnefe {
            cnpj: station().cnpj,
            municipality_ibge: station().municipality_ibge,
            street: "[P25-TEST] Street".into(),
            number: "12A".into(),
            species: 6,
            geocode_level: 1,
            match_reference: "[P25-TEST] exact establishment match".into(),
        },
        _ => panic!("fixture uses external sources"),
    };
    AddressCandidate {
        source,
        components: address("[P25-TEST] Street", "12A"),
        provenance: Provenance {
            reference: "https://example.invalid/synthetic-source".into(),
            edition: "[P25-TEST] edition".into(),
            input_sha256: "a".repeat(64),
            observed_at: "2026-10-01".into(),
        },
        proof,
        reuse: ReuseDecision {
            status: ReuseStatus::Admitted,
            reference: "[P25-TEST] synthetic reuse decision".into(),
        },
    }
}
#[test]
fn every_precedence_band_uses_a_whole_source_object() {
    let mut s = station();
    let r = candidate(AddressSource::Rfb);
    let mut o = candidate(AddressSource::Osm);
    o.components = address("[P25-TEST] OSM Street", "99");
    let c = candidate(AddressSource::Cnefe);
    s.official_address = address("[P25-TEST] Official Street", "");
    assert_eq!(
        select_address(&s, &[o.clone(), c.clone(), r.clone()])
            .unwrap()
            .source,
        Some(AddressSource::Anp)
    );
    s.official_address.clear();
    for (inputs, source) in [
        (vec![o.clone(), c.clone(), r.clone()], AddressSource::Rfb),
        (vec![c.clone(), o.clone()], AddressSource::Osm),
        (vec![c], AddressSource::Cnefe),
        (vec![], AddressSource::Municipality),
    ] {
        let v = select_address(&s, &inputs).unwrap();
        assert_eq!(v.source, Some(source));
        if source == AddressSource::Municipality {
            assert_eq!(v.components.get("municipio"), Some(&s.municipality));
            assert!(!v.components.contains_key("number"));
        } else {
            let expected = inputs.iter().find(|c| c.source == source).unwrap();
            assert_eq!(v.components, expected.components);
            assert_eq!(v.provenance, Some(expected.provenance.clone()));
        }
    }
    let v = select_address(&s, &[r, o]).unwrap();
    assert_eq!(v.components["number"], "12A");
}
#[test]
fn newest_inactive_and_conflicting_evidence_never_revives_or_depends_on_order() {
    let s = station();
    let r = candidate(AddressSource::Rfb);
    let mut newer = r.clone();
    newer.provenance.observed_at = "2026-11-01".into();
    newer.proof = MatchProof::Rfb {
        cnpj: s.cnpj.clone(),
        active: false,
    };
    let o = candidate(AddressSource::Osm);
    let a = select_address(&s, &[r.clone(), o.clone(), newer.clone()]).unwrap();
    let b = select_address(&s, &[newer, o.clone(), r.clone()]).unwrap();
    assert_eq!(a, b);
    assert_eq!(a.source, Some(AddressSource::Osm));
    assert!(a.reviews.iter().any(|r| r.reason == "rfb_inactive"));
    let mut other = r.clone();
    other.components = address("[P25-TEST] Conflicting Street", "22");
    let a = select_address(&s, &[r.clone(), other.clone(), o.clone()]).unwrap();
    let b = select_address(&s, &[o, other, r]).unwrap();
    assert_eq!(a, b);
    assert_eq!(a.source, Some(AddressSource::Osm));
    assert!(a
        .reviews
        .iter()
        .any(|r| r.reason == "ambiguous_latest_address"));
}
#[test]
fn identity_link_rights_and_provenance_gates_skip_with_reasons() {
    for fault in [
        "cnpj",
        "osm_link",
        "cnefe_city",
        "cnefe_number",
        "cnefe_suffix",
        "cnefe_species",
        "cnefe_level",
        "cnefe_link",
        "rights_pending",
        "rights_restricted",
        "rights_reference",
        "hash",
        "date",
        "reference",
    ] {
        let mut c = candidate(if fault.starts_with("cnefe") {
            AddressSource::Cnefe
        } else if fault == "osm_link" {
            AddressSource::Osm
        } else {
            AddressSource::Rfb
        });
        match fault {
            "cnpj" => {
                c.proof = MatchProof::Rfb {
                    cnpj: "91000000000000".into(),
                    active: true,
                }
            }
            "osm_link" => {
                if let MatchProof::Osm { link_reference, .. } = &mut c.proof {
                    link_reference.clear()
                }
            }
            "cnefe_city" => {
                if let MatchProof::Cnefe {
                    municipality_ibge, ..
                } = &mut c.proof
                {
                    *municipality_ibge = "5103403".into()
                }
            }
            "cnefe_number" => c
                .components
                .insert("number".into(), "99".into())
                .map(|_| ())
                .unwrap_or(()),
            "cnefe_suffix" => c
                .components
                .insert("number".into(), "12B".into())
                .map(|_| ())
                .unwrap_or(()),
            "cnefe_species" => {
                if let MatchProof::Cnefe { species, .. } = &mut c.proof {
                    *species = 4
                }
            }
            "cnefe_level" => {
                if let MatchProof::Cnefe { geocode_level, .. } = &mut c.proof {
                    *geocode_level = 5
                }
            }
            "cnefe_link" => {
                if let MatchProof::Cnefe {
                    match_reference, ..
                } = &mut c.proof
                {
                    match_reference.clear()
                }
            }
            "rights_pending" => c.reuse.status = ReuseStatus::Pending,
            "rights_restricted" => c.reuse.status = ReuseStatus::Restricted,
            "rights_reference" => c.reuse.reference.clear(),
            "hash" => c.provenance.input_sha256 = "not-a-hash".into(),
            "date" => c.provenance.observed_at = "2026-02-30".into(),
            "reference" => c.provenance.reference.clear(),
            _ => unreachable!(),
        }
        let v = select_address(&station(), &[c]).unwrap();
        assert_eq!(v.source, Some(AddressSource::Municipality), "{fault}");
        assert!(!v.reviews.is_empty(), "{fault}");
    }
}
#[test]
fn coordinates_codes_unknown_fields_and_incomplete_municipality_are_not_addresses() {
    let mut s = station();
    s.official_address
        .insert("raw".into(), "-12.345, -51.678".into());
    let mut r = candidate(AddressSource::Rfb);
    r.components = address("-12.345, -51.678", "1");
    assert_eq!(
        select_address(&s, &[r]).unwrap().source,
        Some(AddressSource::Municipality)
    );
    s.municipality = "3550308".into();
    assert_eq!(select_address(&s, &[]).unwrap().source, None);
    s.municipality = "[P25-TEST] City".into();
    s.uf = "ZZ".into();
    assert_eq!(select_address(&s, &[]).unwrap().source, None);
    let mut c = candidate(AddressSource::Osm);
    c.components.insert("latitude".into(), "-20".into());
    assert!(!select_address(&station(), &[c]).unwrap().reviews.is_empty());
}
#[test]
fn budgets_invalid_identity_and_source_proof_mismatch_are_refused_or_reviewed() {
    let mut s = station();
    s.cnpj = "not-a-cnpj".into();
    assert!(select_address(&s, &[]).is_err());
    let c = candidate(AddressSource::Rfb);
    assert!(select_address(&station(), &vec![c.clone(); 257]).is_err());
    let mut other = c;
    other.source = AddressSource::Osm;
    let v = select_address(&station(), &[other]).unwrap();
    assert_eq!(v.source, Some(AddressSource::Municipality));
    assert!(v
        .reviews
        .iter()
        .any(|r| r.reason == "source_proof_mismatch"));
}

#[test]
fn ambiguous_references_and_invalid_newest_provenance_cannot_depend_on_input_order() {
    let r = candidate(AddressSource::Rfb);
    let mut other = r.clone();
    other.components = address("[P25-TEST] Other Street", "32");
    other.provenance.reference = "https://example.invalid/other-source-row".into();
    assert_eq!(
        select_address(&station(), &[r.clone(), other.clone()]).unwrap(),
        select_address(&station(), &[other, r.clone()]).unwrap()
    );
    let mut bad = r.clone();
    bad.provenance.observed_at = "2026-99-99".into();
    bad.proof = MatchProof::Rfb {
        cnpj: station().cnpj,
        active: false,
    };
    let result = select_address(&station(), &[r, bad]).unwrap();
    assert_eq!(result.source, Some(AddressSource::Municipality));
}
