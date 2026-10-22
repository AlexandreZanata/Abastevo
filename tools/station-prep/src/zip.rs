//! Bounded ZIP reader for local source archives.
//!
//! The central directory is the authority: member names, sizes and
//! compression methods come from it, the local header must repeat the
//! same name and the produced bytes are checked against the declared
//! CRC32. Member count, expanded size, name length and the ratio between
//! packed and expanded bytes are bounded, so a hostile archive is refused
//! instead of being expanded.

use crate::deflate::{InflateError, Inflater};
use std::io::{self, Cursor, Read};

/// Refusal reasons of the ZIP reader.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ZipError {
    Truncated,
    NoCentralDirectory,
    Zip64Refused,
    Encrypted,
    UnsupportedMethod,
    DuplicateMember,
    MemberBudget,
    ExpansionBudget,
    MissingMember,
    NameMismatch,
    CorruptHeader,
    InvalidName,
    ChecksumMismatch,
}

impl ZipError {
    fn message(self) -> &'static str {
        match self {
            ZipError::Truncated => "truncated archive",
            ZipError::NoCentralDirectory => "central directory not found",
            ZipError::Zip64Refused => "zip64 archive refused",
            ZipError::Encrypted => "encrypted member refused",
            ZipError::UnsupportedMethod => "unsupported compression method",
            ZipError::DuplicateMember => "duplicate archive member",
            ZipError::MemberBudget => "archive member budget exceeded",
            ZipError::ExpansionBudget => "archive expansion budget exceeded",
            ZipError::MissingMember => "archive member not found",
            ZipError::NameMismatch => "archive member name mismatch",
            ZipError::CorruptHeader => "corrupt archive header",
            ZipError::InvalidName => "invalid archive member name",
            ZipError::ChecksumMismatch => "archive member checksum mismatch",
        }
    }
}

impl From<ZipError> for io::Error {
    fn from(error: ZipError) -> Self {
        io::Error::new(io::ErrorKind::InvalidData, error.message())
    }
}

impl From<InflateError> for ZipError {
    fn from(_: InflateError) -> Self {
        ZipError::CorruptHeader
    }
}

/// Bounds applied to one archive.
#[derive(Debug, Clone, Copy)]
pub struct ZipLimits {
    pub max_members: usize,
    pub max_expanded_bytes: u64,
    pub max_member_bytes: u64,
    pub max_name_bytes: usize,
    /// Refuse a member that expands more than this multiple of its packed
    /// size, which bounds a compression bomb independently of the archive
    /// totals.
    pub max_expansion_ratio: u64,
}

impl Default for ZipLimits {
    fn default() -> Self {
        Self {
            max_members: 64,
            max_expanded_bytes: 200 << 20,
            max_member_bytes: 150 << 20,
            max_name_bytes: 512,
            max_expansion_ratio: 200,
        }
    }
}

/// One central directory entry.
#[derive(Debug, Clone)]
pub struct ZipEntry {
    pub name: String,
    pub method: u16,
    pub compressed_size: u64,
    pub uncompressed_size: u64,
    pub crc32: u32,
    pub local_offset: u64,
}

/// Opened archive with its central directory.
pub struct ZipArchive {
    data: Vec<u8>,
    entries: Vec<ZipEntry>,
    limits: ZipLimits,
}

fn read_u16(data: &[u8], offset: usize) -> Option<u16> {
    let slice = data.get(offset..offset + 2)?;
    Some(u16::from_le_bytes([slice[0], slice[1]]))
}

fn read_u32(data: &[u8], offset: usize) -> Option<u32> {
    let slice = data.get(offset..offset + 4)?;
    Some(u32::from_le_bytes([slice[0], slice[1], slice[2], slice[3]]))
}

fn valid_member_name(name: &str) -> bool {
    !name.is_empty()
        && !name.starts_with('/')
        && !name.contains('\\')
        && !name.split('/').any(|part| part == ".." || part == ".")
}

impl ZipArchive {
    /// Read the central directory of a packed archive.
    pub fn open(data: Vec<u8>, limits: ZipLimits) -> Result<ZipArchive, ZipError> {
        let eocd = find_end_of_central_directory(&data).ok_or(ZipError::NoCentralDirectory)?;
        let disk = read_u16(&data, eocd + 4).ok_or(ZipError::Truncated)?;
        let directory_disk = read_u16(&data, eocd + 6).ok_or(ZipError::Truncated)?;
        let entries_on_disk = read_u16(&data, eocd + 8).ok_or(ZipError::Truncated)?;
        let total = read_u16(&data, eocd + 10).ok_or(ZipError::Truncated)?;
        let directory_size = read_u32(&data, eocd + 12).ok_or(ZipError::Truncated)? as u64;
        let directory_offset = read_u32(&data, eocd + 16).ok_or(ZipError::Truncated)? as u64;
        if disk != 0 || directory_disk != 0 || entries_on_disk != total {
            return Err(ZipError::Zip64Refused);
        }
        if total == 0xFFFF || directory_size == 0xFFFF_FFFF || directory_offset == 0xFFFF_FFFF {
            return Err(ZipError::Zip64Refused);
        }
        let total = total as usize;
        if total > limits.max_members {
            return Err(ZipError::MemberBudget);
        }
        let directory_end = directory_offset
            .checked_add(directory_size)
            .ok_or(ZipError::Truncated)?;
        if directory_end > data.len() as u64 {
            return Err(ZipError::Truncated);
        }
        let mut entries: Vec<ZipEntry> = Vec::with_capacity(total);
        let mut expanded_total: u64 = 0;
        let mut offset = directory_offset as usize;
        for _ in 0..total {
            if read_u32(&data, offset) != Some(0x0201_4b50) {
                return Err(ZipError::CorruptHeader);
            }
            let flags = read_u16(&data, offset + 8).ok_or(ZipError::Truncated)?;
            let method = read_u16(&data, offset + 10).ok_or(ZipError::Truncated)?;
            let crc32 = read_u32(&data, offset + 16).ok_or(ZipError::Truncated)?;
            let compressed_size = read_u32(&data, offset + 20).ok_or(ZipError::Truncated)? as u64;
            let uncompressed_size = read_u32(&data, offset + 24).ok_or(ZipError::Truncated)? as u64;
            let name_length = read_u16(&data, offset + 28).ok_or(ZipError::Truncated)? as usize;
            let extra_length = read_u16(&data, offset + 30).ok_or(ZipError::Truncated)? as usize;
            let comment_length = read_u16(&data, offset + 32).ok_or(ZipError::Truncated)? as usize;
            let local_offset = read_u32(&data, offset + 42).ok_or(ZipError::Truncated)? as u64;
            if flags & 0x0001 != 0 {
                return Err(ZipError::Encrypted);
            }
            if flags & 0x0008 != 0 && compressed_size == 0 {
                return Err(ZipError::CorruptHeader);
            }
            let name_start = offset + 46;
            let name_bytes = data
                .get(name_start..name_start + name_length)
                .ok_or(ZipError::Truncated)?;
            let name = String::from_utf8_lossy(name_bytes).into_owned();
            if name.len() > limits.max_name_bytes || !valid_member_name(&name) {
                return Err(ZipError::InvalidName);
            }
            if method != 0 && method != 8 {
                return Err(ZipError::UnsupportedMethod);
            }
            if uncompressed_size > limits.max_member_bytes {
                return Err(ZipError::ExpansionBudget);
            }
            if compressed_size > 0
                && uncompressed_size
                    > compressed_size
                        .saturating_mul(limits.max_expansion_ratio)
                        .max(limits.max_member_bytes)
            {
                return Err(ZipError::ExpansionBudget);
            }
            expanded_total = expanded_total
                .checked_add(uncompressed_size)
                .ok_or(ZipError::ExpansionBudget)?;
            if expanded_total > limits.max_expanded_bytes {
                return Err(ZipError::ExpansionBudget);
            }
            if local_offset as usize + 30 > data.len() {
                return Err(ZipError::Truncated);
            }
            entries.push(ZipEntry {
                name,
                method,
                compressed_size,
                uncompressed_size,
                crc32,
                local_offset,
            });
            offset = name_start + name_length + extra_length + comment_length;
        }
        if entries
            .iter()
            .map(|entry| entry.name.as_str())
            .collect::<std::collections::HashSet<_>>()
            .len()
            != entries.len()
        {
            return Err(ZipError::DuplicateMember);
        }
        Ok(ZipArchive {
            data,
            entries,
            limits,
        })
    }

    /// Central directory entries in archive order.
    pub fn entries(&self) -> &[ZipEntry] {
        &self.entries
    }

    /// Total declared expanded size of every member.
    pub fn expanded_bytes(&self) -> u64 {
        self.entries
            .iter()
            .fold(0u64, |total, entry| total + entry.uncompressed_size)
    }

    /// Open one member for reading. The returned reader produces the
    /// member bytes and refuses a payload that does not match the
    /// declared size or checksum.
    pub fn open_entry(&self, name: &str) -> Result<EntryReader<'_>, ZipError> {
        let entry = self
            .entries
            .iter()
            .find(|entry| entry.name == name)
            .ok_or(ZipError::MissingMember)?;
        let header = entry.local_offset as usize;
        if read_u32(&self.data, header) != Some(0x0403_4b50) {
            return Err(ZipError::CorruptHeader);
        }
        let name_length = read_u16(&self.data, header + 26).ok_or(ZipError::Truncated)? as usize;
        let extra_length = read_u16(&self.data, header + 28).ok_or(ZipError::Truncated)? as usize;
        let name_start = header + 30;
        let local_name = self
            .data
            .get(name_start..name_start + name_length)
            .ok_or(ZipError::Truncated)?;
        if local_name != entry.name.as_bytes() {
            return Err(ZipError::NameMismatch);
        }
        let data_start = name_start + name_length + extra_length;
        let data_end = data_start
            .checked_add(entry.compressed_size as usize)
            .ok_or(ZipError::Truncated)?;
        let payload = self
            .data
            .get(data_start..data_end)
            .ok_or(ZipError::Truncated)?;
        Ok(EntryReader {
            method: entry.method,
            expected_size: entry.uncompressed_size,
            expected_crc: entry.crc32,
            payload,
            stored_position: 0,
            inflater: None,
            produced: 0,
            checksum: 0xFFFF_FFFF,
            finished: false,
        })
    }

    /// Bounds used by this archive.
    pub fn limits(&self) -> &ZipLimits {
        &self.limits
    }
}

/// Reader over one archive member.
pub struct EntryReader<'a> {
    method: u16,
    expected_size: u64,
    expected_crc: u32,
    payload: &'a [u8],
    stored_position: usize,
    inflater: Option<Inflater<Cursor<&'a [u8]>>>,
    produced: u64,
    checksum: u32,
    finished: bool,
}

impl<'a> EntryReader<'a> {
    /// Declared expanded size of the member.
    pub fn expected_size(&self) -> u64 {
        self.expected_size
    }
}

impl Read for EntryReader<'_> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        if buf.is_empty() {
            return Ok(0);
        }
        if self.finished {
            return Ok(0);
        }
        let read = if self.method == 0 {
            let remaining = &self.payload[self.stored_position..];
            let read = remaining.len().min(buf.len());
            buf[..read].copy_from_slice(&remaining[..read]);
            self.stored_position += read;
            if read == 0 {
                self.finished = true;
            }
            read
        } else {
            if self.inflater.is_none() {
                self.inflater = Some(Inflater::new(Cursor::new(self.payload), self.expected_size));
            }
            let inflater = self.inflater.as_mut().expect("inflater present");
            let read = inflater.read(buf)?;
            if read == 0 {
                self.finished = true;
            }
            read
        };
        if read == 0 {
            if self.produced != self.expected_size {
                return Err(ZipError::Truncated.into());
            }
            if !self.checksum != self.expected_crc {
                return Err(ZipError::ChecksumMismatch.into());
            }
            return Ok(0);
        }
        self.produced += read as u64;
        if self.produced > self.expected_size {
            return Err(ZipError::ExpansionBudget.into());
        }
        self.checksum = crc32_update(self.checksum, &buf[..read]);
        Ok(read)
    }
}

fn find_end_of_central_directory(data: &[u8]) -> Option<usize> {
    let minimum = 22;
    if data.len() < minimum {
        return None;
    }
    let search_start = data.len().saturating_sub(minimum + 0xFFFF);
    let mut index = data.len() - minimum;
    loop {
        if read_u32(data, index) == Some(0x0605_4b50) {
            return Some(index);
        }
        if index == search_start {
            return None;
        }
        index -= 1;
    }
}

fn crc32_update(mut crc: u32, bytes: &[u8]) -> u32 {
    for byte in bytes {
        crc ^= u32::from(*byte);
        for _ in 0..8 {
            let mask = if crc & 1 != 0 { 0xEDB8_8320 } else { 0 };
            crc = (crc >> 1) ^ mask;
        }
    }
    crc
}

#[cfg(test)]
pub(crate) mod tests {
    use super::{ZipArchive, ZipError, ZipLimits};
    use std::io::Read;

    /// Minimal ZIP writer used to build deterministic archives for the
    /// reader tests.
    pub(crate) fn stored_archive(members: &[(&str, &[u8])]) -> Vec<u8> {
        let mut out: Vec<u8> = Vec::new();
        let mut central: Vec<u8> = Vec::new();
        let mut offsets: Vec<u32> = Vec::new();
        for (name, payload) in members {
            let offset = out.len() as u32;
            offsets.push(offset);
            out.extend_from_slice(&0x0403_4b50u32.to_le_bytes());
            out.extend_from_slice(&20u16.to_le_bytes());
            out.extend_from_slice(&0u16.to_le_bytes());
            out.extend_from_slice(&0u16.to_le_bytes());
            out.extend_from_slice(&0u16.to_le_bytes());
            out.extend_from_slice(&0u16.to_le_bytes());
            out.extend_from_slice(&crc32_of(payload).to_le_bytes());
            out.extend_from_slice(&(payload.len() as u32).to_le_bytes());
            out.extend_from_slice(&(payload.len() as u32).to_le_bytes());
            out.extend_from_slice(&(name.len() as u16).to_le_bytes());
            out.extend_from_slice(&0u16.to_le_bytes());
            out.extend_from_slice(name.as_bytes());
            out.extend_from_slice(payload);
        }
        for (index, (name, payload)) in members.iter().enumerate() {
            central.extend_from_slice(&0x0201_4b50u32.to_le_bytes());
            central.extend_from_slice(&20u16.to_le_bytes());
            central.extend_from_slice(&20u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&crc32_of(payload).to_le_bytes());
            central.extend_from_slice(&(payload.len() as u32).to_le_bytes());
            central.extend_from_slice(&(payload.len() as u32).to_le_bytes());
            central.extend_from_slice(&(name.len() as u16).to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u16.to_le_bytes());
            central.extend_from_slice(&0u32.to_le_bytes());
            central.extend_from_slice(&offsets[index].to_le_bytes());
            central.extend_from_slice(name.as_bytes());
        }
        let directory_offset = out.len() as u32;
        let directory_size = central.len() as u32;
        out.extend_from_slice(&central);
        out.extend_from_slice(&0x0605_4b50u32.to_le_bytes());
        out.extend_from_slice(&0u16.to_le_bytes());
        out.extend_from_slice(&0u16.to_le_bytes());
        out.extend_from_slice(&(members.len() as u16).to_le_bytes());
        out.extend_from_slice(&(members.len() as u16).to_le_bytes());
        out.extend_from_slice(&directory_size.to_le_bytes());
        out.extend_from_slice(&directory_offset.to_le_bytes());
        out.extend_from_slice(&0u16.to_le_bytes());
        out
    }

    fn crc32_of(bytes: &[u8]) -> u32 {
        let mut crc = 0xFFFF_FFFFu32;
        for byte in bytes {
            crc ^= u32::from(*byte);
            for _ in 0..8 {
                let mask = if crc & 1 != 0 { 0xEDB8_8320 } else { 0 };
                crc = (crc >> 1) ^ mask;
            }
        }
        !crc
    }

    #[test]
    fn reads_stored_members() {
        let archive = stored_archive(&[("a.txt", b"first"), ("b.txt", b"second")]);
        let opened = ZipArchive::open(archive, ZipLimits::default()).unwrap();
        assert_eq!(opened.entries().len(), 2);
        assert_eq!(opened.expanded_bytes(), 11);
        let mut reader = opened.open_entry("b.txt").unwrap();
        let mut payload = String::new();
        reader.read_to_string(&mut payload).unwrap();
        assert_eq!(payload, "second");
    }

    #[test]
    fn refuses_missing_member_and_corrupt_directory() {
        let archive = stored_archive(&[("a.txt", b"first")]);
        let opened = ZipArchive::open(archive.clone(), ZipLimits::default()).unwrap();
        assert!(matches!(
            opened.open_entry("missing.txt"),
            Err(ZipError::MissingMember)
        ));
        let mut broken = archive.clone();
        let length = broken.len();
        broken[length - 6] = 0xFF;
        assert!(ZipArchive::open(broken, ZipLimits::default()).is_err());
    }

    #[test]
    fn refuses_duplicate_members_and_expansion_budget() {
        let archive = stored_archive(&[("a.txt", b"first"), ("a.txt", b"second")]);
        assert!(matches!(
            ZipArchive::open(archive, ZipLimits::default()),
            Err(ZipError::DuplicateMember)
        ));
        let limits = ZipLimits {
            max_expanded_bytes: 2,
            ..ZipLimits::default()
        };
        let archive = stored_archive(&[("a.txt", b"first")]);
        assert!(matches!(
            ZipArchive::open(archive, limits),
            Err(ZipError::ExpansionBudget)
        ));
    }

    #[test]
    fn refuses_a_member_whose_payload_does_not_match_its_checksum() {
        let mut archive = stored_archive(&[("a.txt", b"first")]);
        let position = archive
            .windows(5)
            .position(|window| window == b"first")
            .expect("payload");
        archive[position] = b'F';
        let opened = ZipArchive::open(archive, ZipLimits::default()).unwrap();
        let mut reader = opened.open_entry("a.txt").unwrap();
        let mut payload = Vec::new();
        assert!(reader.read_to_end(&mut payload).is_err());
    }
}
