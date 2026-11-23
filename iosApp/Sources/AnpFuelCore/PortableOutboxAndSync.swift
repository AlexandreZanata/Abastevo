import Foundation

/// Portable durable outbox and offline read state policy (P12-T03 + SEC06-D).
///
/// Mirrors `PortableOutbox.kt` and `PortableSyncState.kt` case-for-case.
/// Zero postfix unwrap operators anywhere in this file.
public enum OutboxState: String, Codable, Equatable, Sendable {
    case queued = "QUEUED"
    case inFlight = "IN_FLIGHT"
    case failed = "FAILED"
    case cancelled = "CANCELLED"
    case acked = "ACKED"
}

public struct OutboxCommand: Equatable, Codable, Identifiable, Sendable {
    public var id: String { commandId }
    public let commandId: String
    public let kind: String
    public let payload: String
    public let revision: Int
    public let state: OutboxState
    public let attempts: Int
    public let nextEligibleTick: Int64
    public let nonce: String

    public init(
        commandId: String,
        kind: String,
        payload: String,
        revision: Int = 1,
        state: OutboxState = .queued,
        attempts: Int = 0,
        nextEligibleTick: Int64 = 0,
        nonce: String = ""
    ) {
        self.commandId = commandId
        self.kind = kind
        self.payload = payload
        self.revision = revision
        self.state = state
        self.attempts = attempts
        self.nextEligibleTick = nextEligibleTick
        self.nonce = nonce
    }
}

public struct OutboxError: Error, Equatable {
    public let code: String
    public init(_ code: String) { self.code = code }
}

public enum PortableOutbox {
    public static let baseDelayTicks: Int64 = 1_000
    public static let maxDelayTicks: Int64 = 3_600_000
    public static let maxDispatchAttempts = 25

    public static func enqueue(
        commands: [OutboxCommand],
        commandId: String,
        kind: String,
        payload: String
    ) throws -> [OutboxCommand] {
        if commandId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            throw OutboxError("outbox.blank-command-id")
        }
        if let idx = commands.firstIndex(where: { $0.commandId == commandId }) {
            let current = commands[idx]
            var next = commands
            next[idx] = OutboxCommand(
                commandId: current.commandId,
                kind: kind,
                payload: payload,
                revision: current.revision + 1,
                state: .queued,
                attempts: current.attempts,
                nextEligibleTick: 0,
                nonce: ""
            )
            return next
        }
        return commands + [OutboxCommand(commandId: commandId, kind: kind, payload: payload)]
    }

    public static func selectNext(
        commands: [OutboxCommand],
        tickMillis: Int64,
        isCancelled: Bool
    ) -> OutboxCommand? {
        if isCancelled { return nil }
        return commands.first { command in
            switch command.state {
            case .queued:
                return true
            case .failed:
                return tickMillis >= command.nextEligibleTick
            case .inFlight, .cancelled, .acked:
                return false
            }
        }
    }

    public static func markInFlight(command: OutboxCommand, nonce: String) throws -> OutboxCommand {
        if nonce.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            throw OutboxError("outbox.blank-nonce")
        }
        return OutboxCommand(
            commandId: command.commandId,
            kind: command.kind,
            payload: command.payload,
            revision: command.revision,
            state: .inFlight,
            attempts: command.attempts,
            nextEligibleTick: command.nextEligibleTick,
            nonce: nonce
        )
    }

    public static func markAcked(
        commands: [OutboxCommand],
        commandId: String,
        revision: Int
    ) -> [OutboxCommand] {
        return commands.filter { !($0.commandId == commandId && $0.revision == revision) }
    }

    public static func markFailed(command: OutboxCommand, tickMillis: Int64) -> OutboxCommand {
        if command.attempts >= maxDispatchAttempts {
            return OutboxCommand(
                commandId: command.commandId,
                kind: command.kind,
                payload: command.payload,
                revision: command.revision,
                state: .failed,
                attempts: maxDispatchAttempts,
                nextEligibleTick: Int64.max,
                nonce: ""
            )
        }
        let attempts = command.attempts + 1
        var delay = baseDelayTicks
        if attempts > 1 {
            for _ in 0..<(attempts - 1) {
                if delay > maxDelayTicks / 2 {
                    delay = maxDelayTicks
                } else {
                    delay = min(delay * 2, maxDelayTicks)
                }
            }
        }
        let spread = delay / 4
        let jitter: Int64
        if spread > 0 {
            let cmdHash = utf16HashCode(command.commandId)
            let mixed = cmdHash ^ Int32(truncatingIfNeeded: attempts &* 0x9E3779B9)
            let hash = Int64(mixed)
            let span = 2 * spread + 1
            jitter = ((hash % span) + span) % span - spread
        } else {
            jitter = 0
        }
        let jittered = min(max(delay + jitter, 0), maxDelayTicks)
        let nextEligible = tickMillis > Int64.max - jittered ? Int64.max : tickMillis + jittered
        return OutboxCommand(
            commandId: command.commandId,
            kind: command.kind,
            payload: command.payload,
            revision: command.revision,
            state: .failed,
            attempts: attempts,
            nextEligibleTick: nextEligible,
            nonce: ""
        )
    }

    public static func cancel(commands: [OutboxCommand], commandId: String) -> [OutboxCommand] {
        return commands.map { command in
            if command.commandId == commandId {
                return OutboxCommand(
                    commandId: command.commandId,
                    kind: command.kind,
                    payload: command.payload,
                    revision: command.revision,
                    state: .cancelled,
                    attempts: command.attempts,
                    nextEligibleTick: command.nextEligibleTick,
                    nonce: ""
                )
            }
            return command
        }
    }

    public static func toRecord(_ command: OutboxCommand) -> [String: String] {
        return [
            "command_id": command.commandId,
            "kind": command.kind,
            "payload": command.payload,
            "revision": String(command.revision),
            "state": command.state.rawValue,
            "attempts": String(command.attempts),
            "next_eligible_tick": String(command.nextEligibleTick),
            "nonce": command.nonce,
        ]
    }

    public static func fromRecord(_ record: [String: String]) throws -> OutboxCommand {
        func field(_ name: String) throws -> String {
            guard let val = record[name] else {
                throw OutboxError("outbox record misses \(name)")
            }
            return val
        }
        let commandId = try field("command_id")
        let kind = try field("kind")
        let payload = try field("payload")
        guard let revision = Int(try field("revision")) else {
            throw OutboxError("outbox record has corrupt revision")
        }
        guard let attempts = Int(try field("attempts")) else {
            throw OutboxError("outbox record has corrupt attempts")
        }
        guard let nextEligibleTick = Int64(try field("next_eligible_tick")) else {
            throw OutboxError("outbox record has corrupt next_eligible_tick")
        }
        guard let state = OutboxState(rawValue: try field("state")) else {
            throw OutboxError("outbox record has corrupt state")
        }
        return OutboxCommand(
            commandId: commandId,
            kind: kind,
            payload: payload,
            revision: revision,
            state: state,
            attempts: attempts,
            nextEligibleTick: nextEligibleTick,
            nonce: record["nonce"] ?? ""
        )
    }

    private static func utf16HashCode(_ text: String) -> Int32 {
        var h: Int32 = 0
        for codeUnit in text.utf16 {
            h = h &* 31 &+ Int32(codeUnit)
        }
        return h
    }
}

public enum ReadOrigin: String, Equatable, Sendable {
    case remoteFresh = "REMOTE_FRESH"
    case cache = "CACHE"
    case cacheStale = "CACHE_STALE"
    case empty = "EMPTY"
}

public struct CachedValue: Equatable, Codable, Sendable {
    public let payload: String
    public let revision: Int64
    public let cachedTick: Int64

    public init(payload: String, revision: Int64, cachedTick: Int64) {
        self.payload = payload
        self.revision = revision
        self.cachedTick = cachedTick
    }
}

public struct ReadResult: Equatable, Sendable {
    public let origin: ReadOrigin
    public let payload: String?
    public let revision: Int64

    public init(origin: ReadOrigin, payload: String?, revision: Int64) {
        self.origin = origin
        self.payload = payload
        self.revision = revision
    }
}

public enum PortableSyncState {
    public static func resolve(
        remotePayload: String?,
        remoteRevision: Int64,
        cache: CachedValue?,
        isOnline: Bool,
        tickMillis: Int64,
        staleAfterMillis: Int64
    ) -> ReadResult {
        if isOnline, let remote = remotePayload {
            return ReadResult(origin: .remoteFresh, payload: remote, revision: remoteRevision)
        }
        if let cached = cache {
            let age = tickMillis - cached.cachedTick
            if age >= 0 && age > staleAfterMillis {
                return ReadResult(origin: .cacheStale, payload: cached.payload, revision: cached.revision)
            }
            return ReadResult(origin: .cache, payload: cached.payload, revision: cached.revision)
        }
        return ReadResult(origin: .empty, payload: nil, revision: 0)
    }
}
