//! Bounded administrative classification for source census, never addresses.
use crate::types::sha256_hex;
use serde::Serialize;
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

pub const MAX_BOUNDARY_BYTES: usize = 8 << 20;
const MAX_POINTS: usize = 2_000_000;
const MAX_GRID_ENTRIES: usize = 1_000_000;
type Point = (f64, f64);
type Ring = Vec<Point>;

#[derive(Debug)]
struct Polygon {
    uf: &'static str,
    rings: Vec<Ring>,
    bounds: [f64; 4],
}

#[derive(Debug)]
pub struct StateIndex {
    pub source_sha256: String,
    extent: [f64; 4],
    polygons: Vec<Polygon>,
    grid: BTreeMap<(i16, i16), Vec<usize>>,
}

#[derive(Debug, Clone, Serialize)]
pub struct StateDecision {
    pub uf: Option<String>,
    pub reason: &'static str,
}

pub const STATES: [&str; 27] = [
    "AC", "AL", "AM", "AP", "BA", "CE", "DF", "ES", "GO", "MA", "MG", "MS", "MT", "PA", "PB", "PE",
    "PI", "PR", "RJ", "RN", "RO", "RR", "RS", "SC", "SE", "SP", "TO",
];

fn state_for_code(code: &str) -> Option<&'static str> {
    match code {
        "11" => Some("RO"),
        "12" => Some("AC"),
        "13" => Some("AM"),
        "14" => Some("RR"),
        "15" => Some("PA"),
        "16" => Some("AP"),
        "17" => Some("TO"),
        "21" => Some("MA"),
        "22" => Some("PI"),
        "23" => Some("CE"),
        "24" => Some("RN"),
        "25" => Some("PB"),
        "26" => Some("PE"),
        "27" => Some("AL"),
        "28" => Some("SE"),
        "29" => Some("BA"),
        "31" => Some("MG"),
        "32" => Some("ES"),
        "33" => Some("RJ"),
        "35" => Some("SP"),
        "41" => Some("PR"),
        "42" => Some("SC"),
        "43" => Some("RS"),
        "50" => Some("MS"),
        "51" => Some("MT"),
        "52" => Some("GO"),
        "53" => Some("DF"),
        _ => None,
    }
}

fn parse_polygon(value: &Value, uf: &'static str, points: &mut usize) -> Result<Polygon, String> {
    let raw_rings = value.as_array().ok_or("boundary polygon array required")?;
    if raw_rings.is_empty() {
        return Err("boundary polygon needs an exterior ring".into());
    }
    let mut rings = Vec::new();
    let mut bounds = [
        f64::INFINITY,
        f64::INFINITY,
        f64::NEG_INFINITY,
        f64::NEG_INFINITY,
    ];
    for raw_ring in raw_rings {
        let raw_points = raw_ring.as_array().ok_or("boundary ring array required")?;
        if !(4..=100_000).contains(&raw_points.len()) {
            return Err("boundary ring budget refused".into());
        }
        *points += raw_points.len();
        if *points > MAX_POINTS {
            return Err("boundary point budget exceeded".into());
        }
        let mut ring = Vec::with_capacity(raw_points.len());
        for raw in raw_points {
            let pair = raw
                .as_array()
                .filter(|v| v.len() >= 2)
                .ok_or("boundary coordinate pair required")?;
            let lon = pair[0].as_f64().ok_or("boundary longitude required")?;
            let lat = pair[1].as_f64().ok_or("boundary latitude required")?;
            if !lon.is_finite()
                || !lat.is_finite()
                || !(-180.0..=180.0).contains(&lon)
                || !(-90.0..=90.0).contains(&lat)
            {
                return Err("boundary coordinate outside WGS84".into());
            }
            bounds[0] = bounds[0].min(lon);
            bounds[1] = bounds[1].min(lat);
            bounds[2] = bounds[2].max(lon);
            bounds[3] = bounds[3].max(lat);
            ring.push((lon, lat));
        }
        if ring.first() != ring.last() {
            return Err("boundary ring must be closed".into());
        }
        rings.push(ring);
    }
    Ok(Polygon { uf, rings, bounds })
}

impl StateIndex {
    pub fn from_bytes(raw: &[u8]) -> Result<Self, String> {
        if raw.len() > MAX_BOUNDARY_BYTES {
            return Err("boundary byte budget exceeded".into());
        }
        let value: Value = serde_json::from_slice(raw).map_err(|_| "invalid boundary JSON")?;
        if value.get("type").and_then(Value::as_str) != Some("FeatureCollection") {
            return Err("boundary feature collection required".into());
        }
        let features = value
            .get("features")
            .and_then(Value::as_array)
            .ok_or("boundary features required")?;
        if features.is_empty() || features.len() > 6000 {
            return Err("boundary feature budget refused".into());
        }
        let mut polygons = Vec::new();
        let mut points = 0;
        for feature in features {
            let code = feature
                .pointer("/properties/codarea")
                .and_then(Value::as_str)
                .ok_or("IBGE municipality code required")?;
            if code.len() != 7 || !code.bytes().all(|b| b.is_ascii_digit()) {
                return Err("invalid IBGE municipality identity".into());
            }
            let uf = state_for_code(&code[..2]).ok_or("unknown IBGE UF prefix")?;
            let geometry = feature
                .get("geometry")
                .ok_or("boundary geometry required")?;
            let coordinates = geometry
                .get("coordinates")
                .ok_or("boundary coordinates required")?;
            match geometry.get("type").and_then(Value::as_str) {
                Some("Polygon") => polygons.push(parse_polygon(coordinates, uf, &mut points)?),
                Some("MultiPolygon") => {
                    let components = coordinates
                        .as_array()
                        .filter(|v| !v.is_empty())
                        .ok_or("boundary multipolygon components required")?;
                    for component in components {
                        polygons.push(parse_polygon(component, uf, &mut points)?);
                    }
                }
                _ => return Err("unsupported boundary geometry".into()),
            }
        }
        let mut grid: BTreeMap<(i16, i16), Vec<usize>> = BTreeMap::new();
        let mut entries = 0;
        for (index, polygon) in polygons.iter().enumerate() {
            let [x0, y0, x1, y1] = polygon.bounds;
            for x in x0.floor() as i16..=x1.floor() as i16 {
                for y in y0.floor() as i16..=y1.floor() as i16 {
                    entries += 1;
                    if entries > MAX_GRID_ENTRIES {
                        return Err("boundary spatial index budget exceeded".into());
                    }
                    grid.entry((x, y)).or_default().push(index);
                }
            }
        }
        let extent = polygons.iter().fold(
            [
                f64::INFINITY,
                f64::INFINITY,
                f64::NEG_INFINITY,
                f64::NEG_INFINITY,
            ],
            |b, p| {
                [
                    b[0].min(p.bounds[0]),
                    b[1].min(p.bounds[1]),
                    b[2].max(p.bounds[2]),
                    b[3].max(p.bounds[3]),
                ]
            },
        );
        Ok(Self {
            extent,
            source_sha256: sha256_hex(raw),
            polygons,
            grid,
        })
    }

    /// Country-wide bounding extent only; absence of a polygon match still stays unassigned.
    pub fn within_country_extent(&self, lat: f64, lon: f64) -> bool {
        lat.is_finite()
            && lon.is_finite()
            && lon >= self.extent[0]
            && lat >= self.extent[1]
            && lon <= self.extent[2]
            && lat <= self.extent[3]
    }

    pub fn classify(&self, lat: f64, lon: f64) -> StateDecision {
        let decision = |uf: Option<&str>, reason| StateDecision {
            uf: uf.map(String::from),
            reason,
        };
        if !lat.is_finite()
            || !lon.is_finite()
            || !(-90.0..=90.0).contains(&lat)
            || !(-180.0..=180.0).contains(&lon)
        {
            return decision(None, "invalid_point");
        }
        let mut matches = BTreeSet::new();
        let mut boundary = false;
        for index in self
            .grid
            .get(&(lon.floor() as i16, lat.floor() as i16))
            .into_iter()
            .flatten()
        {
            let polygon = &self.polygons[*index];
            if lon < polygon.bounds[0]
                || lat < polygon.bounds[1]
                || lon > polygon.bounds[2]
                || lat > polygon.bounds[3]
            {
                continue;
            }
            if polygon.rings.iter().any(|ring| on_boundary(lon, lat, ring)) {
                boundary = true;
                continue;
            }
            if in_ring(lon, lat, &polygon.rings[0])
                && !polygon.rings[1..]
                    .iter()
                    .any(|ring| in_ring(lon, lat, ring))
            {
                matches.insert(polygon.uf);
            }
        }
        if boundary {
            return decision(None, "boundary_ambiguous");
        }
        match matches.len() {
            0 => decision(None, "no_boundary_match"),
            1 => decision(matches.first().copied(), "ibge_polygon"),
            _ => decision(None, "ambiguous_uf"),
        }
    }
}

fn on_boundary(lon: f64, lat: f64, ring: &Ring) -> bool {
    ring.windows(2).any(|edge| {
        let ((x0, y0), (x1, y1)) = (edge[0], edge[1]);
        let cross = (lon - x0) * (y1 - y0) - (lat - y0) * (x1 - x0);
        cross.abs() <= 1e-12
            && lon >= x0.min(x1) - 1e-12
            && lon <= x0.max(x1) + 1e-12
            && lat >= y0.min(y1) - 1e-12
            && lat <= y0.max(y1) + 1e-12
    })
}

fn in_ring(lon: f64, lat: f64, ring: &Ring) -> bool {
    let mut inside = false;
    for edge in ring.windows(2) {
        let ((x0, y0), (x1, y1)) = (edge[0], edge[1]);
        if ((y0 > lat) != (y1 > lat)) && lon < (x1 - x0) * (lat - y0) / (y1 - y0) + x0 {
            inside = !inside;
        }
    }
    inside
}
