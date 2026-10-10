//! Deterministic census-shaped registry generator for RST-07 benchmarks.
//! Fixed seed, skewed municipalities (Zipf over 200 synthetic codes),
//! valid-checksum CNPJs, 2% exact duplicates and 1% older conflicting
//! rows. Output goes to stdout; redirect to a scratch file, never Git.
//!
//! Usage: bench_generate <rows> <seed> > /tmp/station-bench-100k.csv

use std::io::Write;

// xorshift64*: deterministic, no dependency.
struct Rng(u64);

impl Rng {
    fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545F4914F6CDD1D)
    }

    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

fn digit_value(byte: u8) -> u32 {
    if byte <= b'9' {
        (byte - b'0') as u32
    } else {
        (byte - b'A' + 17) as u32
    }
}

/// Valid-checksum CNPJ from a 12-character prefix (digits only here).
fn complete_cnpj(prefix: &[u8; 12]) -> String {
    const W1: [u32; 12] = [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    const W2: [u32; 13] = [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    let sum: u32 = prefix
        .iter()
        .zip(W1)
        .map(|(b, w)| digit_value(*b) * w)
        .sum();
    let first = if sum % 11 >= 2 { 11 - sum % 11 } else { 0 };
    let sum: u32 = prefix
        .iter()
        .zip(W2)
        .map(|(b, w)| digit_value(*b) * w)
        .sum::<u32>()
        + first * W2[12];
    let second = if sum % 11 >= 2 { 11 - sum % 11 } else { 0 };
    format!(
        "{}{first}{second}",
        prefix.iter().map(|b| *b as char).collect::<String>()
    )
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() == 2 && args[1] == "aliases" {
        println!("{{\"entries\": [");
        for city in 0..200 {
            let comma = if city == 199 { "" } else { "," };
            println!(
                "  {{\"uf\": \"SP\", \"ibge\": \"{}\", \"aliases\": [\"CIDADE {:03}\"]}}{comma}",
                3550000 + city,
                city
            );
        }
        println!("]}}");
        return;
    }
    if args.len() != 3 && args.len() != 4 {
        eprintln!("usage: bench_generate <rows> <seed> [normalized] | bench_generate aliases");
        std::process::exit(2);
    }
    // Normalized layout shares the identity/duplicate RNG sequence with
    // the raw layout, so both implementations must accept the same CNPJ
    // multiset from the same seed.
    let normalized = args.len() == 4 && args[3] == "normalized";
    let rows: usize = args[1].parse().expect("rows");
    let seed: u64 = args[2].parse().expect("seed");
    let mut rng = Rng(seed.max(1));
    let out = std::io::stdout();
    let mut out = std::io::BufWriter::new(out.lock());
    if normalized {
        writeln!(
            out,
            "CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO"
        )
        .expect("header");
    } else {
        writeln!(
            out,
            "CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO"
        )
        .expect("header");
    }
    let mut emitted: Vec<String> = Vec::new();
    let mut serial: u64 = 0;
    for _ in 0..rows {
        serial += 1;
        // 2% exact duplicates, 1% older conflicting rows.
        let pick = rng.below(100);
        if pick < 2 && !emitted.is_empty() {
            writeln!(
                out,
                "{}",
                emitted[(rng.next() % emitted.len() as u64) as usize]
            )
            .expect("row");
            continue;
        }
        let mut prefix = [b'0'; 12];
        let mut base = serial;
        for slot in prefix.iter_mut() {
            *slot = b'0' + (base % 10) as u8;
            base /= 10;
        }
        let cnpj = complete_cnpj(&prefix);
        // Zipf-ish skew over 200 synthetic municipalities (clamped so
        // every city resolves in the bench alias table).
        let city = (200 / (1 + rng.below(200))).min(199);
        let (published, effective, address) = if pick == 99 {
            (
                "2023-01-01",
                "2023-01-10",
                format!("AV ANTIGA {}", serial % 900 + 100),
            )
        } else {
            (
                "2024-03-01",
                "2024-03-10",
                format!("RUA GERADA {}", serial % 900 + 100),
            )
        };
        let line = if normalized {
            format!(
                "{cnpj};[RST07-TEST] POSTO {serial:08} LTDA;{};SP;ATIVA;PRC-2024-{serial:06}",
                3550000 + city
            )
        } else {
            format!(
                "SIMP-{serial:08};PRC-2024-{serial:06};{published};[RST07-TEST] POSTO {serial:08} LTDA;{cnpj};{address};;CENTRO;01310900;SP;CIDADE {city:03};BRANCA;{effective}"
            )
        };
        writeln!(out, "{line}").expect("row");
        emitted.push(line);
        // Bound memory: duplicates sample recent rows only.
        if emitted.len() > 4096 {
            emitted.remove(0);
        }
    }
    out.flush().expect("flush");
}
