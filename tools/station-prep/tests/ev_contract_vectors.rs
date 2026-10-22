//! EE-02 reviewed arithmetic oracle, not a source parser or billing engine.
use std::collections::HashSet;

fn reduced(mut numerator: i128, mut denominator: i128) -> (i128, i128) {
    assert!(denominator > 0);
    let (mut a, mut b) = (numerator.abs(), denominator);
    while b != 0 {
        (a, b) = (b, a % b);
    }
    numerator /= a;
    denominator /= a;
    (numerator, denominator)
}

#[test]
fn reviewed_exact_vectors() {
    let csv = include_str!("../../../contracts/testdata/ev/exact-v1.csv");
    let mut seen = HashSet::new();
    for line in csv.lines().skip(1).filter(|s| !s.is_empty()) {
        let row: Vec<_> = line.split(',').collect();
        assert_eq!(row.len(), 8);
        assert!(seen.insert(row[0]));
        let n = |index: usize| row[index].parse::<i128>().expect("reviewed integer");
        let (a, b, c, d) = (n(2), n(3), n(4), n(5));
        let (numerator, denominator) = match row[1] {
            "WH_KM_KWH100" => (a, 10),
            "WH_KM_MJ" => (a * 36, 10000),
            "ENERGY_COST" => (a * b + d * 1000 * c, 1000 * c),
            "COST_KM" => (a * 1000, b),
            "ENERGY100" => (a * 100, b),
            "TIME_COST" => (a * b, 60),
            "IDLE" => (((a - b).max(0) + c - 1) / c * c * d, 60),
            "SUM" => (a + b + c + d, 1),
            "PHEV_COST" => ((a + b) * 1000, c),
            "DIFFERENCE" => (a - b, 1),
            "SOC_MODEL" => (a * b, 10000),
            "ROUND_HALF_UP" => ((2 * a + b) / (2 * b), 1),
            _ => panic!("unknown reviewed operation"),
        };
        assert_eq!(reduced(numerator, denominator), (n(6), n(7)), "{}", row[0]);
    }
    assert_eq!(seen.len(), 24);
}
