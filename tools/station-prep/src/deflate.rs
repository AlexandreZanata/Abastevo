//! Raw DEFLATE (RFC 1951) decoding for bounded archive members.
//!
//! The decoder is deliberately dependency-free: a source archive is read
//! locally and a corrupt or hostile member must be refused instead of
//! expanding without bound. Every block is length-checked, the produced
//! output is capped by the caller and a back-reference may never reach
//! before the start of the produced stream.

use std::io::{self, Read};

/// Refusal reasons of the raw DEFLATE decoder.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum InflateError {
    Truncated,
    InvalidBlockType,
    InvalidCode,
    InvalidLength,
    DistanceTooFar,
    LimitExceeded,
}

impl InflateError {
    fn message(self) -> &'static str {
        match self {
            InflateError::Truncated => "truncated deflate stream",
            InflateError::InvalidBlockType => "invalid deflate block type",
            InflateError::InvalidCode => "invalid huffman code",
            InflateError::InvalidLength => "invalid stored block length",
            InflateError::DistanceTooFar => "back-reference before stream start",
            InflateError::LimitExceeded => "expanded output exceeds the allowed budget",
        }
    }
}

impl From<InflateError> for io::Error {
    fn from(error: InflateError) -> Self {
        io::Error::new(io::ErrorKind::InvalidData, error.message())
    }
}

const WINDOW: usize = 32 * 1024;
const MAX_BITS: usize = 15;

const LENGTH_BASE: [u16; 29] = [
    3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131,
    163, 195, 227, 258,
];
const LENGTH_EXTRA: [u8; 29] = [
    0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0,
];
const DIST_BASE: [u16; 30] = [
    1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537,
    2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577,
];
const DIST_EXTRA: [u8; 30] = [
    0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13,
    13,
];
const CODE_LENGTH_ORDER: [usize; 19] = [
    16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15,
];

/// Canonical Huffman table: symbol counts per code length plus the symbols
/// ordered by code.
#[derive(Debug)]
struct Huffman {
    counts: [u16; MAX_BITS + 1],
    symbols: Vec<u16>,
}

impl Huffman {
    fn build(lengths: &[u8]) -> Result<Huffman, InflateError> {
        let mut counts = [0u16; MAX_BITS + 1];
        for length in lengths {
            if *length as usize > MAX_BITS {
                return Err(InflateError::InvalidCode);
            }
            counts[*length as usize] += 1;
        }
        counts[0] = 0;
        let mut left = 1i32;
        for count in counts.iter().skip(1) {
            left <<= 1;
            left -= i32::from(*count);
            if left < 0 {
                return Err(InflateError::InvalidCode);
            }
        }
        let mut offsets = [0u16; MAX_BITS + 2];
        for length in 1..=MAX_BITS {
            offsets[length + 1] = offsets[length] + counts[length];
        }
        let mut symbols = vec![0u16; lengths.len()];
        for (symbol, length) in lengths.iter().enumerate() {
            if *length != 0 {
                let slot = &mut offsets[*length as usize];
                symbols[*slot as usize] = symbol as u16;
                *slot += 1;
            }
        }
        Ok(Huffman { counts, symbols })
    }
}

/// Streaming raw DEFLATE decoder. The produced bytes are capped by
/// `limit`; exceeding it refuses the member instead of growing without
/// bound.
pub struct Inflater<R: Read> {
    input: R,
    compressed: Vec<u8>,
    compressed_pos: usize,
    input_done: bool,
    bits: u64,
    bit_count: u32,
    state: State,
    final_block: bool,
    literal: Option<Huffman>,
    distance: Option<Huffman>,
    pending: Vec<u8>,
    window: Vec<u8>,
    produced: u64,
    limit: u64,
    failure: Option<InflateError>,
}

#[derive(Debug)]
enum State {
    BlockHeader,
    Stored { remaining: u16 },
    Codes,
    Done,
}

impl<R: Read> Inflater<R> {
    /// Build a decoder over `input` that refuses more than `limit` bytes.
    pub fn new(input: R, limit: u64) -> Self {
        Self {
            input,
            compressed: Vec::new(),
            compressed_pos: 0,
            input_done: false,
            bits: 0,
            bit_count: 0,
            state: State::BlockHeader,
            final_block: false,
            literal: None,
            distance: None,
            pending: Vec::new(),
            window: Vec::new(),
            produced: 0,
            limit,
            failure: None,
        }
    }

    fn fail(&mut self, error: InflateError) -> io::Error {
        self.failure = Some(error);
        io::Error::new(io::ErrorKind::InvalidData, error.message())
    }

    fn fill(&mut self) -> io::Result<()> {
        if self.input_done {
            return Ok(());
        }
        if self.compressed_pos > 0 {
            self.compressed.drain(..self.compressed_pos);
            self.compressed_pos = 0;
        }
        let mut chunk = [0u8; 16 * 1024];
        let read = self.input.read(&mut chunk)?;
        if read == 0 {
            self.input_done = true;
        } else {
            self.compressed.extend_from_slice(&chunk[..read]);
        }
        Ok(())
    }

    fn need_bits(&mut self, count: u32) -> Result<(), io::Error> {
        while self.bit_count < count {
            if self.compressed_pos >= self.compressed.len() {
                self.fill()?;
                if self.compressed_pos >= self.compressed.len() {
                    return Err(self.fail(InflateError::Truncated));
                }
            }
            let byte = self.compressed[self.compressed_pos];
            self.compressed_pos += 1;
            self.bits |= u64::from(byte) << self.bit_count;
            self.bit_count += 8;
        }
        Ok(())
    }

    fn take_bits(&mut self, count: u32) -> Result<u32, io::Error> {
        if count == 0 {
            return Ok(0);
        }
        self.need_bits(count)?;
        let value = (self.bits & ((1u64 << count) - 1)) as u32;
        self.bits >>= count;
        self.bit_count -= count;
        Ok(value)
    }

    fn decode(&mut self, table: &Huffman) -> Result<u16, io::Error> {
        let mut code = 0i32;
        let mut first = 0i32;
        let mut index = 0i32;
        for length in 1..=MAX_BITS {
            code |= self.take_bits(1)? as i32;
            let count = table.counts[length] as i32;
            if code - first < count {
                let symbol = table.symbols[(index + (code - first)) as usize];
                return Ok(symbol);
            }
            index += count;
            first = (first + count) << 1;
            code <<= 1;
        }
        Err(self.fail(InflateError::InvalidCode))
    }

    fn emit(&mut self, byte: u8) -> Result<(), io::Error> {
        if self.produced >= self.limit {
            return Err(self.fail(InflateError::LimitExceeded));
        }
        self.produced += 1;
        self.pending.push(byte);
        self.window.push(byte);
        if self.window.len() > 2 * WINDOW {
            let excess = self.window.len() - WINDOW;
            self.window.drain(..excess);
        }
        Ok(())
    }

    fn copy_back(&mut self, distance: usize, length: usize) -> Result<(), io::Error> {
        if distance > self.window.len() || distance == 0 {
            return Err(self.fail(InflateError::DistanceTooFar));
        }
        for _ in 0..length {
            let byte = self.window[self.window.len() - distance];
            self.emit(byte)?;
        }
        Ok(())
    }

    fn fixed_tables(&self) -> (Huffman, Huffman) {
        let mut literal_lengths = [0u8; 288];
        for (symbol, slot) in literal_lengths.iter_mut().enumerate() {
            *slot = match symbol {
                0..=143 => 8,
                144..=255 => 9,
                256..=279 => 7,
                _ => 8,
            };
        }
        let distance_lengths = [5u8; 30];
        (
            Huffman::build(&literal_lengths).expect("fixed literal table"),
            Huffman::build(&distance_lengths).expect("fixed distance table"),
        )
    }

    fn dynamic_tables(&mut self) -> Result<(Huffman, Huffman), io::Error> {
        let literal_count = self.take_bits(5)? as usize + 257;
        let distance_count = self.take_bits(5)? as usize + 1;
        let code_count = self.take_bits(4)? as usize + 4;
        if literal_count > 286 || distance_count > 30 {
            return Err(self.fail(InflateError::InvalidCode));
        }
        let mut code_lengths = [0u8; 19];
        for index in 0..code_count {
            code_lengths[CODE_LENGTH_ORDER[index]] = self.take_bits(3)? as u8;
        }
        let code_table = Huffman::build(&code_lengths)?;
        let mut lengths = vec![0u8; literal_count + distance_count];
        let mut index = 0usize;
        while index < lengths.len() {
            let symbol = self.decode(&code_table)?;
            match symbol {
                0..=15 => {
                    lengths[index] = symbol as u8;
                    index += 1;
                }
                16 => {
                    if index == 0 {
                        return Err(self.fail(InflateError::InvalidCode));
                    }
                    let previous = lengths[index - 1];
                    let repeat = 3 + self.take_bits(2)? as usize;
                    if index + repeat > lengths.len() {
                        return Err(self.fail(InflateError::InvalidCode));
                    }
                    for _ in 0..repeat {
                        lengths[index] = previous;
                        index += 1;
                    }
                }
                17 => {
                    let repeat = 3 + self.take_bits(3)? as usize;
                    if index + repeat > lengths.len() {
                        return Err(self.fail(InflateError::InvalidCode));
                    }
                    index += repeat;
                }
                18 => {
                    let repeat = 11 + self.take_bits(7)? as usize;
                    if index + repeat > lengths.len() {
                        return Err(self.fail(InflateError::InvalidCode));
                    }
                    index += repeat;
                }
                _ => return Err(self.fail(InflateError::InvalidCode)),
            }
        }
        let literal = Huffman::build(&lengths[..literal_count])?;
        let distance = Huffman::build(&lengths[literal_count..])?;
        Ok((literal, distance))
    }

    fn step(&mut self) -> io::Result<bool> {
        match self.state {
            State::Done => Ok(false),
            State::BlockHeader => {
                let last = self.take_bits(1)?;
                let kind = self.take_bits(2)?;
                match kind {
                    0 => {
                        self.bits = 0;
                        self.bit_count = 0;
                        let mut header = [0u8; 4];
                        for slot in header.iter_mut() {
                            if self.compressed_pos >= self.compressed.len() {
                                self.fill()?;
                                if self.compressed_pos >= self.compressed.len() {
                                    return Err(self.fail(InflateError::Truncated));
                                }
                            }
                            *slot = self.compressed[self.compressed_pos];
                            self.compressed_pos += 1;
                        }
                        let length = u16::from_le_bytes([header[0], header[1]]);
                        let check = u16::from_le_bytes([header[2], header[3]]);
                        if length != !check {
                            return Err(self.fail(InflateError::InvalidLength));
                        }
                        self.state = State::Stored { remaining: length };
                    }
                    1 => {
                        let (literal, distance) = self.fixed_tables();
                        self.literal = Some(literal);
                        self.distance = Some(distance);
                        self.state = State::Codes;
                    }
                    2 => {
                        let (literal, distance) = self.dynamic_tables()?;
                        self.literal = Some(literal);
                        self.distance = Some(distance);
                        self.state = State::Codes;
                    }
                    _ => return Err(self.fail(InflateError::InvalidBlockType)),
                }
                self.final_block = last == 1;
                Ok(true)
            }
            State::Stored { remaining } => {
                if remaining == 0 {
                    self.state = if self.final_block {
                        State::Done
                    } else {
                        State::BlockHeader
                    };
                    return Ok(true);
                }
                if self.compressed_pos >= self.compressed.len() {
                    self.fill()?;
                    if self.compressed_pos >= self.compressed.len() {
                        return Err(self.fail(InflateError::Truncated));
                    }
                }
                let byte = self.compressed[self.compressed_pos];
                self.compressed_pos += 1;
                self.emit(byte)?;
                self.state = State::Stored {
                    remaining: remaining - 1,
                };
                Ok(true)
            }
            State::Codes => {
                let literal = self
                    .literal
                    .take()
                    .ok_or_else(|| self.fail(InflateError::InvalidCode))?;
                let distance = self
                    .distance
                    .take()
                    .ok_or_else(|| self.fail(InflateError::InvalidCode))?;
                let outcome = self.decode_codes(&literal, &distance);
                self.literal = Some(literal);
                self.distance = Some(distance);
                outcome?;
                Ok(true)
            }
        }
    }

    fn decode_codes(&mut self, literal: &Huffman, distance: &Huffman) -> io::Result<()> {
        loop {
            let symbol = self.decode(literal)?;
            match symbol {
                0..=255 => {
                    self.emit(symbol as u8)?;
                }
                256 => {
                    self.state = if self.final_block {
                        State::Done
                    } else {
                        State::BlockHeader
                    };
                    return Ok(());
                }
                257..=285 => {
                    let index = symbol as usize - 257;
                    let extra = u32::from(LENGTH_EXTRA[index]);
                    let length = usize::from(LENGTH_BASE[index]) + self.take_bits(extra)? as usize;
                    let distance_symbol = self.decode(distance)? as usize;
                    if distance_symbol >= DIST_BASE.len() {
                        return Err(self.fail(InflateError::InvalidCode));
                    }
                    let distance_extra = u32::from(DIST_EXTRA[distance_symbol]);
                    let back = usize::from(DIST_BASE[distance_symbol])
                        + self.take_bits(distance_extra)? as usize;
                    self.copy_back(back, length)?;
                }
                _ => return Err(self.fail(InflateError::InvalidCode)),
            }
        }
    }
}

impl<R: Read> Read for Inflater<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        if buf.is_empty() {
            return Ok(0);
        }
        while self.pending.is_empty() {
            if !self.step()? {
                return Ok(0);
            }
        }
        let take = buf.len().min(self.pending.len());
        buf[..take].copy_from_slice(&self.pending[..take]);
        self.pending.drain(..take);
        Ok(take)
    }
}

/// Decode one complete raw DEFLATE member into memory, refusing more
/// than `limit` produced bytes.
pub fn inflate(input: &[u8], limit: u64) -> Result<Vec<u8>, InflateError> {
    let mut decoder = Inflater::new(input, limit);
    let mut out = Vec::new();
    let mut chunk = [0u8; 16 * 1024];
    loop {
        let read = match decoder.read(&mut chunk) {
            Ok(read) => read,
            Err(_) => return Err(decoder.failure.unwrap_or(InflateError::Truncated)),
        };
        if read == 0 {
            break;
        }
        out.extend_from_slice(&chunk[..read]);
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::{inflate, InflateError, Inflater};
    use std::io::Read;

    /// Raw DEFLATE streams (RFC 1951, raw deflate without a zlib header)
    /// produced by an independent implementation.
    const STORED_BLOCK: &[u8] = &[0x01, 0x05, 0x00, 0xfa, 0xff, b'h', b'e', b'l', b'l', b'o'];
    const FIXED_BLOCK: &[u8] = &[0x4b, 0x4c, 0x4a, 0x4e, 0xc4, 0x8b, 0x00];
    const DYNAMIC_BLOCK: &[u8] = &[
        0x7d, 0xd2, 0x41, 0x0a, 0x83, 0x40, 0x10, 0x44, 0xd1, 0x7d, 0x4e, 0xd1, 0x47, 0x48, 0x55,
        0x12, 0x35, 0xe7, 0x09, 0x2d, 0x04, 0x74, 0x06, 0x54, 0xf0, 0xfa, 0x92, 0x7d, 0xfa, 0xaf,
        0xff, 0xaa, 0x1e, 0xb5, 0x7c, 0x5b, 0xc6, 0x3d, 0xfa, 0x1c, 0xd9, 0xe6, 0xbe, 0x7d, 0x72,
        0xcd, 0x76, 0xc4, 0xd6, 0xcf, 0xfd, 0xb6, 0xfc, 0x92, 0xea, 0xe4, 0x3a, 0x3d, 0xea, 0xf4,
        0xac, 0xd3, 0xab, 0x4e, 0x43, 0x9d, 0xc6, 0x3a, 0x4d, 0x75, 0x7a, 0xc3, 0x64, 0xe2, 0x00,
        0x0f, 0x01, 0x88, 0x40, 0x44, 0x40, 0x22, 0x30, 0x11, 0xa0, 0x08, 0x54, 0x04, 0x2c, 0x02,
        0x17, 0x83, 0x8b, 0xe9, 0x27, 0xe0, 0x62, 0x70, 0x31, 0xb8, 0x18, 0x5c, 0x0c, 0x2e, 0x06,
        0x17, 0x83, 0x8b, 0xff, 0xbb, 0x5c,
    ];
    /// Fixed Huffman block with one back-reference inside the produced
    /// stream: literal 'a', literal 'b', then three bytes copied from two
    /// bytes back.
    const BACK_REFERENCE: &[u8] = &[0x4b, 0x4c, 0x02, 0x42, 0x00];
    /// Fixed Huffman block whose back-reference reaches before the start of
    /// the produced stream.
    const BACK_REFERENCE_TOO_FAR: &[u8] = &[0x4b, 0x04, 0x42, 0x00];

    fn dynamic_plain() -> Vec<u8> {
        (0..30)
            .flat_map(|index| format!("line {index} of enforcement rows\n").into_bytes())
            .collect()
    }

    #[test]
    fn decodes_stored_fixed_and_dynamic_blocks() {
        assert_eq!(inflate(STORED_BLOCK, 1024).unwrap(), b"hello".to_vec());
        assert_eq!(
            inflate(FIXED_BLOCK, 1024).unwrap(),
            b"abcabcabcabcabcabcabcabcabcabcabc".to_vec()
        );
        assert_eq!(inflate(DYNAMIC_BLOCK, 4096).unwrap(), dynamic_plain());
    }

    #[test]
    fn copies_back_references_inside_the_produced_stream() {
        assert_eq!(inflate(BACK_REFERENCE, 1024).unwrap(), b"ababa".to_vec());
    }

    #[test]
    fn refuses_output_beyond_the_budget() {
        assert_eq!(inflate(FIXED_BLOCK, 4), Err(InflateError::LimitExceeded));
    }

    #[test]
    fn refuses_truncated_and_invalid_blocks() {
        assert_eq!(
            inflate(&STORED_BLOCK[..4], 1024),
            Err(InflateError::Truncated)
        );
        assert_eq!(
            inflate(&[0x07, 0x00], 1024),
            Err(InflateError::InvalidBlockType)
        );
        assert_eq!(
            inflate(&[0x01, 0x05, 0x00, 0x00, 0x00], 1024),
            Err(InflateError::InvalidLength)
        );
        assert_eq!(
            inflate(BACK_REFERENCE_TOO_FAR, 1024),
            Err(InflateError::DistanceTooFar)
        );
    }

    #[test]
    fn streaming_reader_reports_the_refusal_kind() {
        let mut decoder = Inflater::new(BACK_REFERENCE_TOO_FAR, 1024);
        let mut out = Vec::new();
        let error = decoder.read_to_end(&mut out).unwrap_err();
        assert_eq!(error.kind(), std::io::ErrorKind::InvalidData);
    }
}
