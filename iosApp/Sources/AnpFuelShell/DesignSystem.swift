import SwiftUI
import AnpFuelCore
#if canImport(UIKit)
import UIKit
#endif

/// Abastevo "Energia" Light & Dark Design System and iOS native chrome components.
public enum AbastevoTheme {
    public static let ink = Color(red: 0, green: 0, blue: 0)
    public static let paper = Color.white
    public static let mist = Color(red: 0.953, green: 0.953, blue: 0.941) // #F3F3F0
    public static let darkBg = Color(red: 0.043, green: 0.047, blue: 0.051) // #0B0C0D
    public static let darkSurface = Color(red: 0.102, green: 0.106, blue: 0.118) // #1A1B1E
    public static let darkElevated = Color(red: 0.137, green: 0.141, blue: 0.157) // #232428
    public static let chromeControl = Color(red: 0.122, green: 0.122, blue: 0.122) // #1F1F1F
    public static let accentLight = Color(red: 0.039, green: 0.478, blue: 0.275) // #0A7A46
    public static let accentDark = Color(red: 0.290, green: 0.871, blue: 0.502) // #4ADE80
    public static let warningLight = Color(red: 0.478, green: 0.294, blue: 0) // #7A4B00
    public static let warningDark = Color(red: 0.961, green: 0.773, blue: 0.420) // #F5C56B
    public static let errorRed = Color(red: 0.702, green: 0.149, blue: 0.118) // #B3261E

    public static func screenBackground(_ colorScheme: ColorScheme) -> Color {
        return colorScheme == .dark ? darkBg : mist
    }

    public static func cardBackground(_ colorScheme: ColorScheme) -> Color {
        return colorScheme == .dark ? darkSurface : paper
    }

    public static func cardBorder(_ colorScheme: ColorScheme) -> Color {
        return colorScheme == .dark ? Color(white: 0.22) : Color(red: 0.871, green: 0.871, blue: 0.847)
    }

    public static func accent(_ colorScheme: ColorScheme) -> Color {
        return colorScheme == .dark ? accentDark : accentLight
    }

    public static func fuelColor(_ product: FuelProduct, colorScheme: ColorScheme) -> Color {
        let hex = colorScheme == .dark ? product.tintHexDark : product.tintHexLight
        let r = Double((hex >> 16) & 0xFF) / 255.0
        let g = Double((hex >> 8) & 0xFF) / 255.0
        let b = Double(hex & 0xFF) / 255.0
        return Color(red: r, green: g, blue: b)
    }

    public static func formatBrlMilli(_ milli: Int64) -> String {
        guard (PortableMoney.minMilli...PortableMoney.maxMilli).contains(milli) else { return "—" }
        return (Decimal(milli) / 1_000).formatted(.currency(code: "BRL").precision(.fractionLength(3)))
    }

    public static func formatCentavos(_ centavos: Int64) -> String {
        (Decimal(centavos) / 100).formatted(.currency(code: "BRL").precision(.fractionLength(2)))
    }
}

public enum IosHaptics {
    public static func lightTap() {
        #if os(iOS) && canImport(UIKit)
        let generator = UIImpactFeedbackGenerator(style: .light)
        generator.impactOccurred()
        #endif
    }

    public static func success() {
        #if os(iOS) && canImport(UIKit)
        let generator = UINotificationFeedbackGenerator()
        generator.notificationOccurred(.success)
        #endif
    }

    public static func warning() {
        #if os(iOS) && canImport(UIKit)
        let generator = UINotificationFeedbackGenerator()
        generator.notificationOccurred(.warning)
        #endif
    }
}

public struct EnergiaCard<Content: View>: View {
    @Environment(\.colorScheme) private var colorScheme
    private let content: () -> Content

    public init(@ViewBuilder content: @escaping () -> Content) {
        self.content = content
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            content()
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(AbastevoTheme.cardBackground(colorScheme))
        .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .stroke(AbastevoTheme.cardBorder(colorScheme), lineWidth: 1)
        )
    }
}

public struct AbastevoTopChromeBar: View {
    let locationLabel: String
    let weekLabel: String
    let isOffline: Bool
    let signedInAlias: String?
    let onLocationTap: UnitAction
    let onWeekTap: UnitAction
    let onSettingsTap: UnitAction
    let onProfileTap: UnitAction

    public typealias UnitAction = () -> Void

    public var body: some View {
        VStack(spacing: 8) {
            HStack(spacing: 10) {
                HStack(spacing: 6) {
                    Text(verbatim: "A")
                        .accessibilityHidden(true)
                        .font(.system(size: 16, weight: .black, design: .rounded))
                        .foregroundStyle(AbastevoTheme.accentDark)
                        .frame(width: 26, height: 26)
                        .background(AbastevoTheme.chromeControl)
                        .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
                    Text(String(localized: "brand.name"))
                        .font(.system(size: 18, weight: .bold, design: .rounded))
                        .foregroundStyle(.white)
                }
                Spacer()
                Button(action: onSettingsTap) {
                    Image(systemName: "gearshape.fill")
                        .foregroundStyle(.white)
                        .frame(width: 34, height: 34)
                        .background(AbastevoTheme.chromeControl)
                        .clipShape(Circle())
                }
                .accessibilityLabel(String(localized: "android.nav_settings"))

                Button(action: onProfileTap) {
                    HStack(spacing: 5) {
                        Image(systemName: signedInAlias == nil ? "person.crop.circle" : "person.crop.circle.fill")
                            .foregroundStyle(signedInAlias == nil ? .white : AbastevoTheme.accentDark)
                        if let alias = signedInAlias {
                            Text(alias)
                                .font(.caption2.bold())
                                .foregroundStyle(.white)
                                .lineLimit(1)
                        }
                    }
                    .padding(.horizontal, signedInAlias == nil ? 8 : 10)
                    .frame(height: 34)
                    .background(AbastevoTheme.chromeControl)
                    .clipShape(Capsule())
                }
                .accessibilityLabel(String(localized: "android.nav_profile"))
            }

            HStack(spacing: 8) {
                Button(action: onLocationTap) {
                    HStack(spacing: 6) {
                        Image(systemName: "mappin.and.ellipse")
                            .foregroundStyle(AbastevoTheme.accentDark)
                        Text(locationLabel)
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(.white)
                            .lineLimit(1)
                        Spacer(minLength: 2)
                        Image(systemName: "chevron.down")
                            .font(.caption2)
                            .foregroundStyle(.gray)
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(AbastevoTheme.chromeControl)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                }

                Button(action: onWeekTap) {
                    HStack(spacing: 4) {
                        Image(systemName: "calendar")
                            .font(.caption)
                        Text(weekLabel)
                            .font(.caption.weight(.medium))
                            .lineLimit(1)
                    }
                    .foregroundStyle(.white)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 8)
                    .background(AbastevoTheme.chromeControl)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                }
            }

            if isOffline {
                HStack(spacing: 6) {
                    Image(systemName: "wifi.slash")
                        .font(.caption2)
                    Text(String(localized: "network.offline_work"))
                        .font(.caption2)
                }
                .foregroundStyle(AbastevoTheme.warningDark)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .padding(.bottom, 12)
        .background(AbastevoTheme.ink)
    }
}

public enum AppTab: String, CaseIterable, Identifiable {
    case explore
    case community
    case map
    case profile

    public var id: String { rawValue }

    public var systemImage: String {
        switch self {
        case .explore: return "safari"
        case .community: return "person.3"
        case .map: return "map"
        case .profile: return "person.crop.circle"
        }
    }

    public var selectedSystemImage: String {
        switch self {
        case .explore: return "safari.fill"
        case .community: return "person.3.fill"
        case .map: return "map.fill"
        case .profile: return "person.crop.circle.fill"
        }
    }

    public var title: String {
        switch self {
        case .explore: return String(localized: "android.nav_explore")
        case .community: return String(localized: "android.nav_community")
        case .map: return String(localized: "android.nav_map")
        case .profile: return String(localized: "android.nav_profile")
        }
    }
}

public struct AbastevoBottomTabBar: View {
    let selectedTab: AppTab
    let onSelectTab: (AppTab) -> Void

    public var body: some View {
        HStack(spacing: 6) {
            ForEach(AppTab.allCases) { tab in
                let isSelected = tab == selectedTab
                Button {
                    IosHaptics.lightTap()
                    onSelectTab(tab)
                } label: {
                    VStack(spacing: 4) {
                        Image(systemName: isSelected ? tab.selectedSystemImage : tab.systemImage)
                            .font(.system(size: 18, weight: isSelected ? .bold : .regular))
                        Text(tab.title)
                            .font(.system(size: 11, weight: isSelected ? .bold : .medium))
                            .lineLimit(1)
                    }
                    .foregroundStyle(isSelected ? AbastevoTheme.ink : Color(white: 0.75))
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                    .background(isSelected ? AbastevoTheme.mist : Color.clear)
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                }
                .accessibilityAddTraits(isSelected ? [.isSelected] : [])
            }
        }
        .padding(.horizontal, 10)
        .padding(.top, 8)
        .padding(.bottom, 6)
        .background(AbastevoTheme.ink)
    }
}

public struct FuelProductChipsBar: View {
    @Environment(\.colorScheme) private var colorScheme
    let selectedFuel: FuelProduct
    let onSelect: (FuelProduct) -> Void

    public var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(FuelProduct.allCases) { product in
                    let isSelected = product == selectedFuel
                    let tint = AbastevoTheme.fuelColor(product, colorScheme: colorScheme)
                    Button {
                        IosHaptics.lightTap()
                        onSelect(product)
                    } label: {
                        HStack(spacing: 6) {
                            Circle()
                                .fill(tint)
                                .frame(width: 8, height: 8)
                            Text(product.localizedName)
                                .font(.subheadline.weight(isSelected ? .bold : .medium))
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .foregroundStyle(isSelected ? (colorScheme == .dark ? AbastevoTheme.ink : .white) : .primary)
                        .background(isSelected ? (colorScheme == .dark ? AbastevoTheme.mist : AbastevoTheme.ink) : AbastevoTheme.cardBackground(colorScheme))
                        .clipShape(Capsule())
                        .overlay(
                            Capsule().stroke(AbastevoTheme.cardBorder(colorScheme), lineWidth: isSelected ? 0 : 1)
                        )
                    }
                }
            }
            .padding(.horizontal, 16)
        }
    }
}

public struct SourceTrustBadge: View {
    let isOfficial: Bool
    let label: String

    public var body: some View {
        HStack(spacing: 4) {
            Image(systemName: isOfficial ? "checkmark.seal.fill" : "camera.viewfinder")
                .font(.caption2)
            Text(label)
                .font(.caption2.bold())
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 4)
        .foregroundStyle(isOfficial ? Color.blue : AbastevoTheme.accentLight)
        .background((isOfficial ? Color.blue : AbastevoTheme.accentLight).opacity(0.14))
        .clipShape(Capsule())
    }
}
