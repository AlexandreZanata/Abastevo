//! Pure deterministic source hierarchy. No nearest-parent inference or count sums.
use crate::ev_sites::{SiteKind, SourceFootprint, SourceSite};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};

type Point = (f64, f64);
const EPS: f64 = 1e-12;
const GRID_DEGREES: f64 = 0.01;
const MAX_GRID_ENTRIES: usize = 100_000;
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SiteWithEquipment {
    pub site: SourceSite,
    pub equipment: Vec<SourceSite>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct HierarchyReview {
    pub key: String,
    pub reason: String,
    pub candidates: Vec<String>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct EquipmentAssignment {
    pub key: String,
    pub root_key: String,
    pub basis: String,
    pub via: Vec<String>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Hierarchy {
    pub sites: Vec<SiteWithEquipment>,
    pub review: Vec<HierarchyReview>,
    pub assignments: Vec<EquipmentAssignment>,
}
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Location {
    Outside,
    Inside,
    Boundary,
}
fn parse_point(site: &SourceSite) -> Result<Point, &'static str> {
    let lat = site
        .point
        .latitude_decimal
        .parse::<f64>()
        .map_err(|_| "invalid source point")?;
    let lon = site
        .point
        .longitude_decimal
        .parse::<f64>()
        .map_err(|_| "invalid source point")?;
    if !lat.is_finite()
        || !lon.is_finite()
        || !(-90.0..=90.0).contains(&lat)
        || !(-180.0..=180.0).contains(&lon)
    {
        return Err("invalid source point");
    }
    Ok((lon, lat))
}
fn ring(raw: &[[String; 2]]) -> Option<Vec<Point>> {
    raw.iter()
        .map(|p| p[0].parse().ok().zip(p[1].parse().ok()))
        .collect()
}
fn cross(a: Point, b: Point, c: Point) -> f64 {
    (b.0 - a.0) * (c.1 - a.1) - (b.1 - a.1) * (c.0 - a.0)
}
fn on_segment(p: Point, a: Point, b: Point) -> bool {
    cross(a, b, p).abs() <= EPS
        && p.0 >= a.0.min(b.0) - EPS
        && p.0 <= a.0.max(b.0) + EPS
        && p.1 >= a.1.min(b.1) - EPS
        && p.1 <= a.1.max(b.1) + EPS
}
fn intersects(a: Point, b: Point, c: Point, d: Point) -> bool {
    let (x, y, z, w) = (
        cross(a, b, c),
        cross(a, b, d),
        cross(c, d, a),
        cross(c, d, b),
    );
    ((x > EPS && y < -EPS) || (x < -EPS && y > EPS))
        && ((z > EPS && w < -EPS) || (z < -EPS && w > EPS))
        || on_segment(a, c, d)
        || on_segment(b, c, d)
        || on_segment(c, a, b)
        || on_segment(d, a, b)
}
fn simple(raw: &[[String; 2]]) -> bool {
    let Some(points) = ring(raw) else {
        return false;
    };
    if points.len() < 4 {
        return false;
    }
    let origin = points[0];
    let area: f64 = points
        .windows(2)
        .map(|p| {
            (p[0].0 - origin.0) * (p[1].1 - origin.1) - (p[1].0 - origin.0) * (p[0].1 - origin.1)
        })
        .sum();
    if area.abs() < 1e-18 {
        return false;
    }
    let count = points.len() - 1;
    for i in 0..count {
        for j in i + 1..count {
            if j == i + 1 || (i == 0 && j == count - 1) {
                continue;
            }
            if intersects(points[i], points[i + 1], points[j], points[j + 1]) {
                return false;
            }
        }
    }
    true
}
fn locate_ring(point: Point, points: &[Point]) -> Location {
    if points.windows(2).any(|p| on_segment(point, p[0], p[1])) {
        return Location::Boundary;
    }
    let mut inside = false;
    for edge in points.windows(2) {
        let (a, b) = (edge[0], edge[1]);
        if (a.1 > point.1) != (b.1 > point.1)
            && point.0 < (b.0 - a.0) * (point.1 - a.1) / (b.1 - a.1) + a.0
        {
            inside = !inside;
        }
    }
    if inside {
        Location::Inside
    } else {
        Location::Outside
    }
}
fn valid_footprint(shape: &SourceFootprint) -> bool {
    if !shape.valid() {
        return false;
    }
    for polygon in &shape.polygons {
        if !simple(&polygon.outer) || polygon.holes.iter().any(|r| !simple(r)) {
            return false;
        }
        let Some(outer) = ring(&polygon.outer) else {
            return false;
        };
        for hole in &polygon.holes {
            let Some(h) = ring(hole) else {
                return false;
            };
            if locate_ring(h[0], &outer) != Location::Inside {
                return false;
            }
            if outer
                .windows(2)
                .any(|a| h.windows(2).any(|b| intersects(a[0], a[1], b[0], b[1])))
            {
                return false;
            }
        }
    }
    true
}
fn locate(point: Point, shape: &SourceFootprint) -> Location {
    let mut result = Location::Outside;
    for polygon in &shape.polygons {
        let outer = ring(&polygon.outer).expect("validated source ring");
        match locate_ring(point, &outer) {
            Location::Outside => continue,
            Location::Boundary => return Location::Boundary,
            Location::Inside => {}
        }
        let mut hole = false;
        for raw in &polygon.holes {
            match locate_ring(point, &ring(raw).expect("validated source hole")) {
                Location::Boundary => return Location::Boundary,
                Location::Inside => {
                    hole = true;
                    break;
                }
                Location::Outside => {}
            }
        }
        if !hole {
            result = Location::Inside;
        }
    }
    result
}
fn full_child_inside(child: &SourceSite, parent: &SourceFootprint) -> bool {
    let Some(shape) = &child.footprint else {
        return true;
    };
    if !valid_footprint(shape) {
        return false;
    }
    let mut strict = false;
    for polygon in &shape.polygons {
        let points = ring(&polygon.outer).expect("validated child ring");
        for p in &points {
            match locate(*p, parent) {
                Location::Outside => return false,
                Location::Inside => strict = true,
                Location::Boundary => {}
            }
        }
        // A non-convex parent's boundary must not cut through a child edge.
        for parent_poly in &parent.polygons {
            for raw in std::iter::once(&parent_poly.outer).chain(&parent_poly.holes) {
                let boundary = ring(raw).expect("validated parent ring");
                for a in points.windows(2) {
                    for b in boundary.windows(2) {
                        let (x, y, z, w) = (
                            cross(a[0], a[1], b[0]),
                            cross(a[0], a[1], b[1]),
                            cross(b[0], b[1], a[0]),
                            cross(b[0], b[1], a[1]),
                        );
                        if ((x > EPS && y < -EPS) || (x < -EPS && y > EPS))
                            && ((z > EPS && w < -EPS) || (z < -EPS && w > EPS))
                        {
                            return false;
                        }
                    }
                }
            }
        }
    }
    strict
}
fn roots(
    key: &str,
    records: &BTreeMap<String, SourceSite>,
    parents: &BTreeMap<String, BTreeSet<String>>,
    errors: &BTreeMap<String, String>,
    path: &mut BTreeSet<String>,
    memo: &mut BTreeMap<String, Result<BTreeSet<String>, String>>,
) -> Result<BTreeSet<String>, String> {
    if let Some(value) = memo.get(key) {
        return value.clone();
    }
    if path.contains(key) {
        return Err("parent_cycle".into());
    }
    if path.len() >= 64 {
        return Err("parent_depth_limit".into());
    }
    if let Some(reason) = errors.get(key) {
        return Err(reason.clone());
    }
    path.insert(key.into());
    let record = &records[key];
    let candidates = parents.get(key);
    let result = if candidates.is_some_and(|v| !v.is_empty()) {
        let mut found = BTreeSet::new();
        let mut error = None;
        for parent in candidates.unwrap() {
            match roots(parent, records, parents, errors, path, memo) {
                Ok(r) => {
                    found.extend(r);
                    if found.len() > 64 {
                        error = Some("parent_candidate_budget".into());
                        break;
                    }
                }
                Err(reason) => {
                    error = Some(reason);
                    break;
                }
            }
        }
        if let Some(reason) = error {
            Err(reason)
        } else {
            Ok(found)
        }
    } else if record.site_marker && record.kind != SiteKind::ChargePoint {
        Ok(BTreeSet::from([key.into()]))
    } else {
        Err("unparented_equipment".into())
    };
    path.remove(key);
    memo.insert(key.into(), result.clone());
    result
}

pub fn resolve_hierarchy(input: &[SourceSite]) -> Result<Hierarchy, &'static str> {
    if input.len() > 205000 {
        return Err("source record budget exceeded");
    }
    let mut records = BTreeMap::new();
    for record in input {
        parse_point(record)?;
        if !record.site_marker && !record.equipment_marker {
            return Err("missing source role");
        }
        if records.insert(record.key.clone(), record.clone()).is_some() {
            return Err("duplicate source identity");
        }
    }
    let mut grid: BTreeMap<(i32, i32), BTreeSet<String>> = BTreeMap::new();
    let mut shapes = BTreeMap::new();
    let mut review = Vec::new();
    let mut entries = 0;
    for (key, site) in &records {
        if site.source.source != "OSM" || !site.site_marker {
            continue;
        }
        let Some(shape) = &site.footprint else {
            continue;
        };
        if !valid_footprint(shape) {
            review.push(HierarchyReview {
                key: key.clone(),
                reason: "invalid_parent_geometry".into(),
                candidates: vec![],
            });
            continue;
        }
        let points: Vec<_> = shape
            .polygons
            .iter()
            .flat_map(|p| ring(&p.outer).expect("validated source ring"))
            .collect();
        let bounds = points.iter().fold(
            [
                f64::INFINITY,
                f64::INFINITY,
                f64::NEG_INFINITY,
                f64::NEG_INFINITY,
            ],
            |b, p| [b[0].min(p.0), b[1].min(p.1), b[2].max(p.0), b[3].max(p.1)],
        );
        for x in
            (bounds[0] / GRID_DEGREES).floor() as i32..=(bounds[2] / GRID_DEGREES).floor() as i32
        {
            for y in (bounds[1] / GRID_DEGREES).floor() as i32
                ..=(bounds[3] / GRID_DEGREES).floor() as i32
            {
                entries += 1;
                if entries > MAX_GRID_ENTRIES {
                    return Err("source footprint grid budget exceeded");
                }
                grid.entry((x, y)).or_default().insert(key.clone());
            }
        }
        shapes.insert(key.clone(), shape);
    }
    let mut parents = BTreeMap::new();
    let mut errors = BTreeMap::new();
    for (key, child) in &records {
        if !child.equipment_marker {
            continue;
        }
        let mut found = BTreeSet::new();
        if !child.parent_keys.is_empty() {
            for parent in &child.parent_keys {
                if parent != key
                    && records
                        .get(parent)
                        .is_some_and(|s| s.source.source == "OSM" && s.site_marker)
                {
                    found.insert(parent.clone());
                }
            }
            if found.is_empty() {
                errors.insert(key.clone(), "missing_declared_parent".into());
            }
        } else {
            let point = parse_point(child)?;
            let cell = (
                (point.0 / GRID_DEGREES).floor() as i32,
                (point.1 / GRID_DEGREES).floor() as i32,
            );
            let mut boundary = false;
            for parent in grid.get(&cell).into_iter().flatten() {
                if parent == key {
                    continue;
                }
                let shape = shapes[parent];
                match locate(point, shape) {
                    Location::Inside => {
                        if full_child_inside(child, shape) {
                            found.insert(parent.clone());
                        }
                    }
                    Location::Boundary => boundary = true,
                    Location::Outside => {}
                }
            }
            if boundary {
                errors.insert(key.clone(), "parent_boundary_ambiguous".into());
            }
        }
        if found.len() > 64 {
            errors.insert(key.clone(), "parent_candidate_budget".into());
        }
        parents.insert(key.clone(), found);
    }
    let mut groups: BTreeMap<String, SiteWithEquipment> = BTreeMap::new();
    let mut assignments = Vec::new();
    let mut memo = BTreeMap::new();
    for (key, record) in &records {
        if !record.equipment_marker {
            groups
                .entry(key.clone())
                .or_insert_with(|| SiteWithEquipment {
                    site: record.clone(),
                    equipment: vec![],
                });
            continue;
        }
        match roots(
            key,
            &records,
            &parents,
            &errors,
            &mut BTreeSet::new(),
            &mut memo,
        ) {
            Ok(found) if found.len() == 1 => {
                let root = found.first().unwrap();
                let group = groups
                    .entry(root.clone())
                    .or_insert_with(|| SiteWithEquipment {
                        site: records[root].clone(),
                        equipment: vec![],
                    });
                if root != key {
                    group.equipment.push(record.clone());
                    assignments.push(EquipmentAssignment {
                        key: key.clone(),
                        root_key: root.clone(),
                        basis: if record.parent_keys.is_empty() {
                            "source_footprint"
                        } else {
                            "source_membership"
                        }
                        .into(),
                        via: parents
                            .get(key)
                            .map(|v| v.iter().cloned().collect())
                            .unwrap_or_default(),
                    });
                }
            }
            Ok(found) => review.push(HierarchyReview {
                key: key.clone(),
                reason: "ambiguous_parent".into(),
                candidates: found.into_iter().collect(),
            }),
            Err(reason) => {
                if record.site_marker
                    && record.kind != SiteKind::ChargePoint
                    && record.parent_keys.is_empty()
                {
                    groups
                        .entry(key.clone())
                        .or_insert_with(|| SiteWithEquipment {
                            site: record.clone(),
                            equipment: vec![],
                        });
                }
                review.push(HierarchyReview {
                    key: key.clone(),
                    reason,
                    candidates: parents
                        .get(key)
                        .map(|v| v.iter().cloned().collect())
                        .unwrap_or_default(),
                });
            }
        }
    }
    for group in groups.values_mut() {
        group.equipment.sort_by(|a, b| a.key.cmp(&b.key));
    }
    review.sort_by(|a, b| a.key.cmp(&b.key).then(a.reason.cmp(&b.reason)));
    review.dedup();
    assignments.sort_by(|a, b| a.key.cmp(&b.key));
    Ok(Hierarchy {
        assignments,
        sites: groups.into_values().collect(),
        review,
    })
}
