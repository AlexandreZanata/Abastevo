import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Premium status and App Store purchase verification (B-BR-STORE-01..10).
/// The server decides access; this file only carries its answers.

public struct PremiumTierWire: Decodable, Equatable, Sendable {
    public let tier: String
    public let source: String
    public let validUntil: String?
}

public struct PremiumStoreProductWire: Decodable, Equatable, Sendable {
    public let productId: String
    public let planIds: [String]
}

public struct PremiumStoreOfferWire: Decodable, Equatable, Sendable {
    public let provider: String
    public let products: [PremiumStoreProductWire]
    public let accountBinding: String
    public let managementUrl: String
}

public struct PremiumStatusWire: Decodable, Equatable, Sendable {
    public let policyVersion: Int
    public let state: String
    public let reason: String
    public let capabilities: [String]
    public let validUntil: String?
    public let retainedDataAccess: Bool
    public let paymentsConfigured: Bool
    public let tiers: [PremiumTierWire]
    public let storeOffers: [PremiumStoreOfferWire]

    private enum CodingKeys: String, CodingKey {
        case policyVersion, state, reason, capabilities, validUntil
        case retainedDataAccess, paymentsConfigured, tiers, storeOffers
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        policyVersion = try values.decode(Int.self, forKey: .policyVersion)
        state = try values.decode(String.self, forKey: .state)
        reason = try values.decode(String.self, forKey: .reason)
        capabilities = try values.decodeIfPresent([String].self, forKey: .capabilities) ?? []
        validUntil = try values.decodeIfPresent(String.self, forKey: .validUntil)
        retainedDataAccess = try values.decode(Bool.self, forKey: .retainedDataAccess)
        paymentsConfigured = try values.decode(Bool.self, forKey: .paymentsConfigured)
        tiers = try values.decodeIfPresent([PremiumTierWire].self, forKey: .tiers) ?? []
        storeOffers = try values.decodeIfPresent([PremiumStoreOfferWire].self, forKey: .storeOffers) ?? []
    }

    /// True while any source grants the Premium bundle.
    public var isPremiumActive: Bool {
        return !capabilities.isEmpty && (state == "ACTIVE" || state == "CANCELED" || state == "GRACE")
    }

    /// The App Store offer, if the server sells through the App Store.
    public var appStoreOffer: PremiumStoreOfferWire? {
        return storeOffers.first { $0.provider == PremiumStoreBilling.appStoreProvider }
    }
}

public struct PremiumReceiptWire: Decodable, Equatable, Sendable {
    public let operationId: String
    public let status: PremiumStatusWire
    public let replayed: Bool
}

public enum PremiumApiError: Error, Equatable {
    case invalid
    case authRequired
    case denied
    case retry
    case quota
    case unavailable
}

public enum PremiumPurchaseKind: String, Sendable {
    case verify
    case restore
}

/// Adds the owner's device-key signature to a purchase command. The signed
/// request transport of the identity module implements it.
public protocol PremiumRequestSigner {
    func signedHeaders(method: String, path: String, body: Data) async throws -> [String: String]
}

/// What the Premium screen may offer for the App Store (B-BR-STORE-04/10).
public enum PremiumPurchaseAvailability: Equatable {
    /// Premium is already active from some source; no second purchase.
    case active
    /// The server does not sell through the App Store now.
    case unavailable
    /// Products and the appAccountToken to attach to the purchase.
    case available(productIds: [String], appAccountToken: UUID)
}

/// Whether an unfinished StoreKit transaction may be finished after the
/// server answered (B-BR-STORE-08).
public enum PremiumFinishDecision: Equatable {
    case finish
    case keepForRetry
}

public enum PremiumStoreBilling {
    public static let appStoreProvider = "app_store"
    public static let statusPath = "/v1/premium/status"

    public static func purchasePath(_ kind: PremiumPurchaseKind) -> String {
        return "/v1/premium/purchases/" + kind.rawValue
    }

    public static func availability(for status: PremiumStatusWire) -> PremiumPurchaseAvailability {
        if status.isPremiumActive {
            return .active
        }
        guard let offer = status.appStoreOffer,
              let token = UUID(uuidString: offer.accountBinding),
              !offer.products.isEmpty else {
            return .unavailable
        }
        let ids = offer.products.map(\.productId).filter { !$0.isEmpty }
        return ids.isEmpty ? .unavailable : .available(productIds: ids, appAccountToken: token)
    }

    /// Accepted or definitively refused purchases are finished; transport,
    /// session, quota and provider failures keep the transaction for retry.
    public static func finishDecision(for result: Result<PremiumReceiptWire, PremiumApiError>) -> PremiumFinishDecision {
        switch result {
        case .success:
            return .finish
        case .failure(let error):
            return error == .denied || error == .invalid ? .finish : .keepForRetry
        }
    }

    /// Maps an HTTP status and the server error code to a client error.
    public static func error(httpStatus: Int, code: String?) -> PremiumApiError {
        switch httpStatus {
        case 400: return .invalid
        case 401: return .authRequired
        case 403: return .denied
        case 409: return .retry
        case 429: return .quota
        default:
            return code == "subscription.retry-verification" ? .retry : .unavailable
        }
    }

    /// The exact JSON body of a purchase command, signed byte for byte.
    public static func purchaseBody(
        familyId: String,
        accessToken: String,
        operationId: String,
        originalTransactionId: UInt64
    ) throws -> Data {
        let body: [String: String] = [
            "family_id": familyId,
            "access_token": accessToken,
            "operation_id": operationId.lowercased(),
            "purchase_token": String(originalTransactionId),
            "provider": appStoreProvider,
        ]
        return try JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
    }

    public static func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return decoder
    }
}

extension AbastevoHttpClient {
    public func fetchPremiumStatus(familyId: String, accessToken: String) async throws -> PremiumStatusWire {
        let body = try JSONSerialization.data(
            withJSONObject: ["family_id": familyId, "access_token": accessToken],
            options: [.sortedKeys]
        )
        let data = try await premiumPost(path: PremiumStoreBilling.statusPath, body: body, headers: [:])
        do {
            return try PremiumStoreBilling.decoder().decode(PremiumStatusWire.self, from: data)
        } catch {
            throw PremiumApiError.unavailable
        }
    }

    /// Sends an App Store purchase to the server. The caller reuses
    /// operationId when retrying the same transaction.
    public func submitAppStorePurchase(
        kind: PremiumPurchaseKind,
        originalTransactionId: UInt64,
        operationId: String,
        familyId: String,
        accessToken: String,
        signer: PremiumRequestSigner
    ) async throws -> PremiumReceiptWire {
        let path = PremiumStoreBilling.purchasePath(kind)
        let body = try PremiumStoreBilling.purchaseBody(
            familyId: familyId,
            accessToken: accessToken,
            operationId: operationId,
            originalTransactionId: originalTransactionId
        )
        let headers: [String: String]
        do {
            headers = try await signer.signedHeaders(method: "POST", path: path, body: body)
        } catch {
            throw PremiumApiError.authRequired
        }
        let data = try await premiumPost(path: path, body: body, headers: headers)
        do {
            return try PremiumStoreBilling.decoder().decode(PremiumReceiptWire.self, from: data)
        } catch {
            throw PremiumApiError.unavailable
        }
    }

    private func premiumPost(path: String, body: Data, headers: [String: String]) async throws -> Data {
        guard let url = environment.join(path) else { throw PremiumApiError.invalid }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        for (name, value) in headers {
            request.setValue(value, forHTTPHeaderField: name)
        }
        let exchange: (Data, URLResponse)
        do {
            exchange = try await session.data(for: request)
        } catch {
            throw PremiumApiError.unavailable
        }
        let (data, response) = exchange
        guard data.count <= 512 * 1024, let http = response as? HTTPURLResponse else {
            throw PremiumApiError.unavailable
        }
        guard (200..<300).contains(http.statusCode) else {
            var code: String?
            if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
               let error = json["error"] as? [String: Any] {
                code = error["code"] as? String
            }
            throw PremiumStoreBilling.error(httpStatus: http.statusCode, code: code)
        }
        return data
    }
}
