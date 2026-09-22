import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../../../core/widgets/arch_background.dart';
import '../providers/asset_connection_provider.dart';

/// Screen 1 of Banks Flow: Shows Bank Accounts (Image 1) in clean light mode.
/// Allows checking/unchecking accounts, viewing consent info bottom sheet (Image 2),
/// and proceeding to account fetching.
class BanksLinkingScreen extends ConsumerStatefulWidget {
  const BanksLinkingScreen({super.key});

  @override
  ConsumerState<BanksLinkingScreen> createState() => _BanksLinkingScreenState();
}

class _BanksLinkingScreenState extends ConsumerState<BanksLinkingScreen> {
  final List<String> _selectedMoreBanks = [];
  final List<String> _popularBanks = [
    'HDFC Bank',
    'ICICI Bank',
    'State Bank of India',
    'Axis Bank',
    'Kotak Mahindra Bank',
    'Punjab National Bank',
    'Bank of Baroda',
    'Canara Bank',
    'Union Bank of India',
    'Bank of India',
    'Indian Bank',
    'Central Bank of India',
    'Indian Overseas Bank',
    'UCO Bank',
    'Bank of Maharashtra',
    'Punjab & Sind Bank',
  ];

  @override
  void initState() {
    super.initState();
    // Ensure banks are shown in state if not already populated
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final state = ref.read(assetConnectionProvider);
      if (state.bankAccounts.isEmpty) {
        ref.read(assetConnectionProvider.notifier).showFoundBanks();
      }
    });
  }

  void _showConsentBottomSheet(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
      ),
      builder: (sheetContext) => _ConsentBottomSheet(
        onUnderstood: () => Navigator.pop(sheetContext),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(assetConnectionProvider);
    final notifier = ref.read(assetConnectionProvider.notifier);
    // GoRouter extra: true means we returned after a partial link
    final returnMode = GoRouterState.of(context).extra == true;
    // In return mode: CTA active when any NEW (unlinked) account is selected
    final hasSelected = state.bankAccounts.any(
      (b) => b.isSelected && !b.isLinked,
    );
    final hasAnyLinked = state.bankAccounts.any((b) => b.isLinked);

    String ctaLabel;
    bool ctaActive;
    VoidCallback? onCtaTap;

    if (_selectedMoreBanks.isNotEmpty) {
      ctaLabel = 'PROCEED';
      ctaActive = true;
      onCtaTap = () {
        for (final bank in _selectedMoreBanks) {
          notifier.searchAndAddBank(bank);
        }
        setState(() {
          _selectedMoreBanks.clear();
        });
        context.push('/banks-searching');
      };
    } else if (hasSelected) {
      ctaLabel = 'APPROVE AND CONNECT';
      ctaActive = true;
      onCtaTap = () {
        notifier.startBankLinking();
        context.go('/');
      };
    } else {
      ctaLabel = 'APPROVE AND CONNECT';
      ctaActive = false;
      onCtaTap = null;
    }

    return Scaffold(
      backgroundColor: const Color(0xFFFFFFFF),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 24.0, vertical: 16.0),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 420),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                    const SizedBox(height: 24),
                    // Title
                    const Text(
                      'Get latest balance and transactions',
                      style: TextStyle(
                        fontFamily: 'SpaceGrotesk',
                        color: Color(0xFF0F172A),
                        fontSize: 28,
                        fontWeight: FontWeight.w700,
                        letterSpacing: -1.0,
                        height: 1.15,
                      ),
                    ),
                    const SizedBox(height: 22),

                    // Subtitle with link
                    Wrap(
                      children: [
                        const Text(
                          'SECURELY TRACK ON ASTRA. REVOKE THIS ANYTIME. ',
                          style: TextStyle(
                            fontFamily: 'DMSans',
                            color: Color(0xFF9CA3AF),
                            fontSize: 10,
                            height: 1.4,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        GestureDetector(
                          onTap: () => _showConsentBottomSheet(context),
                          child: const Text(
                            'KNOW MORE.',
                            style: TextStyle(
                              fontFamily: 'DMSans',
                              color: Color(0xFF0F172A),
                              fontSize: 10,
                              fontWeight: FontWeight.w600,
                              height: 1.4,
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 24),

                    // Stats Row
                    Row(
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        _buildStatItem('UPDATED', 'Daily'),
                        _buildStatItem('VALID FOR', '1 year'),
                        _buildStatItem('ACCESS', '1 Month'),
                      ],
                    ),
                    const SizedBox(height: 32),
                    
                    Row(
                      children: const [
                        Text(
                          'YOUR ACCOUNTS',
                          style: TextStyle(
                            fontFamily: 'DMSans',
                            color: Color(0xFF9CA3AF),
                            fontSize: 10,
                            fontWeight: FontWeight.w600,
                            letterSpacing: 1.2,
                          ),
                        ),
                        SizedBox(width: 6),
                        Icon(Icons.unfold_more, size: 16, color: Color(0xFF64748B)),
                      ],
                    ),
                    const SizedBox(height: 8),

                    // Bank Accounts List
                    Flexible(
                      child: SingleChildScrollView(
                        child: Column(
                          children: state.bankAccounts.map((bank) {
                            return _buildBankCard(
                              bank: bank,
                              onTap: () => notifier.toggleBankSelection(bank.id),
                            );
                          }).toList(),
                        ),
                      ),
                    ),

                    const SizedBox(height: 46),

                    // Connect More Accounts Label
                    Row(
                      children: const [
                        Text(
                          'CONNECT MORE ACCOUNTS',
                          style: TextStyle(
                            fontFamily: 'DMSans',
                            color: Color(0xFF9CA3AF),
                            fontSize: 10,
                            fontWeight: FontWeight.w600,
                            letterSpacing: 1.2,
                          ),
                        ),
                        SizedBox(width: 6),
                        Icon(Icons.unfold_more, size: 16, color: Color(0xFF64748B)),
                      ],
                    ),
                    const SizedBox(height: 8),

                    Expanded(
                      child: ListView.builder(
                        padding: EdgeInsets.zero,
                          itemCount: _popularBanks.length,
                          itemBuilder: (context, index) {
                          final bankName = _popularBanks[index];
                          final isSelected = _selectedMoreBanks.contains(bankName);
                          return Container(
                            margin: const EdgeInsets.only(bottom: 8),
                            decoration: BoxDecoration(
                              color: const Color(0xFFFFFFFF),
                              border: Border.all(
                                color: const Color(0xFFE2E8F0),
                                width: 1.0,
                              ),
                              borderRadius: BorderRadius.zero,
                              boxShadow: [
                                BoxShadow(
                                  color: const Color(0xFF0F172A).withValues(alpha: 0.02),
                                  blurRadius: 4,
                                  offset: const Offset(0, 2),
                                ),
                              ],
                            ),
                            child: ListTile(
                              dense: true,
                              contentPadding: const EdgeInsets.symmetric(
                                horizontal: 16,
                                vertical: 2,
                              ),
                              leading: _buildBankLogoWidget(bankName, size: 24),
                              title: Text(
                                bankName.toUpperCase(),
                                style: TextStyle(
                                  fontFamily: 'DMSans',
                                  color: isSelected
                                      ? const Color(0xFF0F172A)
                                      : const Color(0xFF475569),
                                  fontSize: 11,
                                  fontWeight: isSelected
                                      ? FontWeight.w700
                                      : FontWeight.w600,
                                ),
                              ),
                              trailing: Container(
                                width: 18,
                                height: 18,
                                decoration: BoxDecoration(
                                  color: isSelected ? null : Colors.transparent,
                                  gradient: isSelected
                                      ? const LinearGradient(
                                          colors: [
                                            Color(0xFFFFFFFF),
                                            Color(0xFF5BA1F7),
                                            Color(0xFF031E6B),
                                            Color(0xFF241714),
                                          ],
                                          stops: [0.0, 0.25, 0.7, 1.0],
                                          begin: Alignment.topLeft,
                                          end: Alignment.bottomRight,
                                        )
                                      : null,
                                  border: isSelected
                                      ? null
                                      : Border.all(
                                          color: const Color(0xFFCBD5E1),
                                          width: 1.5,
                                        ),
                                ),
                                child: isSelected
                                    ? const Icon(Icons.check,
                                        color: Colors.white, size: 12)
                                    : null,
                              ),
                              onTap: () {
                                setState(() {
                                  if (_selectedMoreBanks.contains(bankName)) {
                                    _selectedMoreBanks.remove(bankName);
                                  } else {
                                    _selectedMoreBanks.add(bankName);
                                  }
                                });
                              },
                            ),
                          );
                        },
                      ),
                    ),


                      ],
                    ),
                  ),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 0),
              child: Center(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 420),
                  child: Column(
                    children: [
                      GestureDetector(
                        onTap: onCtaTap,
                        child: Container(
                          width: double.infinity,
                          padding: const EdgeInsets.symmetric(vertical: 15),
                          decoration: BoxDecoration(
                            gradient: LinearGradient(
                              colors: ctaActive
                                  ? const [
                                      Color(0xFFFFFFFF),
                                      Color(0xFF5BA1F7),
                                      Color(0xFF031E6B),
                                      Color(0xFF241714),
                                    ]
                                  : const [
                                      Color(0xFFF3F4F6),
                                      Color(0xFFD1D5DB),
                                      Color(0xFF9CA3AF),
                                      Color(0xFF6B7280),
                                    ],
                              stops: const [0.0, 0.25, 0.7, 1.0],
                              begin: Alignment.topLeft,
                              end: Alignment.bottomRight,
                            ),
                          ),
                          child: Stack(
                            alignment: Alignment.center,
                            children: [
                              Text(
                                ctaLabel,
                                style: const TextStyle(
                                  fontFamily: 'DMSans',
                                  color: Colors.white,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                  letterSpacing: 1.0,
                                ),
                              ),
                              const Positioned(
                                right: 20,
                                child: Icon(
                                  Icons.arrow_forward_rounded,
                                  color: Colors.white,
                                  size: 18,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                      const SizedBox(height: 16),
                      // Secondary action
                      Center(
                        child: GestureDetector(
                          onTap: () {
                            if (hasAnyLinked || returnMode) {
                              // User explicitly done — mark state complete and go home
                              notifier.finishAssetConnection();
                              context.go('/');
                            } else {
                              notifier.skipBanks();
                              context.go('/');
                            }
                          },
                          child: Padding(
                            padding: const EdgeInsets.symmetric(vertical: 8),
                            child: Text(
                              (hasAnyLinked || returnMode)
                                  ? "DON'T WANT TO CONNECT ANY MORE"
                                  : 'DENY',
                              style: const TextStyle(
                                fontFamily: 'DMSans',
                                color: Color(0xFF64748B),
                                fontSize: 13,
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),

            // Footer
            _buildFooter(),
          ],
        ),
      ),
    );
  }

  Widget _buildStatItem(String label, String value) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontFamily: 'DMSans',
            color: Color(0xFF9CA3AF),
            fontSize: 10,
            fontWeight: FontWeight.w600,
            letterSpacing: 0.8,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          value,
          style: const TextStyle(
            fontFamily: 'DMSans',
            color: Color(0xFF0F172A),
            fontSize: 10,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    );
  }

  String? _getBankLogoAsset(String bankName) {
    final name = bankName.toLowerCase();
    
    if (name.contains('hdfc')) {
      return 'lib/core/images/hdfc_logo.webp';
    } else if (name.contains('icici')) return 'lib/core/images/icici.webp';
    else if (name.contains('state bank') || name.contains('sbi')) return 'lib/core/images/sbi_logo.webp';
    else if (name.contains('axis')) return 'lib/core/images/axis_logo.webp';
    else if (name.contains('kotak')) return 'lib/core/images/kotak.webp';
    else if (name.contains('punjab national') || name.contains('pnb')) return 'lib/core/images/pnb.webp';
    else if (name.contains('bank of baroda') || name.contains('bob')) return 'lib/core/images/bankofbaroda_logo.webp';
    else if (name.contains('canara')) return 'lib/core/images/canara_logo.webp';
    else if (name.contains('union')) return 'lib/core/images/uniobank_logo.webp';
    else if (name.contains('indusind')) return 'lib/core/images/indusind_logo.webp';
    else if (name.contains('yes')) return 'lib/core/images/yesbank_logo.webp';
    else if (name.contains('uco')) return 'lib/core/images/uco_bank_logo.webp';
    else if (name.contains('punjab & sind') || name.contains('punjab and sind')) return 'lib/core/images/punjab_sindh_bank_logo.webp';
    else if (name.contains('indian overseas bank')) return 'lib/core/images/indian_overseas_bank_logo.webp';
    else if (name.contains('indian bank')) return 'lib/core/images/indian_bank_logo.webp';
    else if (name.contains('maharashtra')) return 'lib/core/images/bank_of_maharashtra_logo.webp';
    else if (name.contains('bank of india') || name.contains('boi')) return 'lib/core/images/bankofindia_logo.webp';
    
    return null;
  }

  Widget _buildFallbackLogo(double size) {
    return Container(
      width: size,
      height: size,
      decoration: const BoxDecoration(
        color: Color(0xFFF1F5F9),
        shape: BoxShape.circle,
      ),
      child: Center(
        child: Icon(
          Icons.account_balance,
          size: size * 0.6,
          color: Color(0xFF94A3B8),
        ),
      ),
    );
  }

  Widget _buildBankLogoWidget(String bankName, {double size = 30}) {
    final assetPath = _getBankLogoAsset(bankName);
    
    if (assetPath != null) {
      return ClipRRect(
        borderRadius: BorderRadius.circular(size / 2),
        child: Container(
          color: Colors.white,
          child: Image.asset(
            assetPath,
            width: size,
            height: size,
            fit: BoxFit.contain,
            errorBuilder: (context, error, stackTrace) {
              return _buildFallbackLogo(size);
            },
          ),
        ),
      );
    }
    
    return _buildFallbackLogo(size);
  }

  Widget _buildBankCard({
    required BankAccountItem bank,
    required VoidCallback? onTap,
  }) {
    final accountLast4 = bank.accountNumber.length >= 4
        ? bank.accountNumber.substring(bank.accountNumber.length - 4)
        : bank.accountNumber;
    final accountType = bank.accountNumber.split(' ').first.toUpperCase();
    final isLinked = bank.isLinked;

    return GestureDetector(
      onTap: onTap,
      child: Container(
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        decoration: const BoxDecoration(
          color: Color(0xFFFFFFFF),
        ),
        child: Row(
          children: [
            // Bank Icon
            _buildBankLogoWidget(bank.bankName, size: 30),
            const SizedBox(width: 14),

            // Name & Account Number
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    bank.bankName.toUpperCase(),
                    style: const TextStyle(
                      fontFamily: 'DMSans',
                      color: Color(0xFF0F172A),
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    '.. $accountLast4 • $accountType',
                    style: const TextStyle(
                      fontFamily: 'DMMono',
                      color: Color(0xFF9CA3AF),
                      fontSize: 10,
                    ),
                  ),
                ],
              ),
            ),

            if (isLinked) ...[
            // Linked badge
              const SizedBox(width: 8),
              const Icon(
                Icons.check_circle_rounded,
                color: Color(0xFF10B981),
                size: 20,
              ),
            ] else ...[
              Container(
                width: 18,
                height: 18,
                decoration: BoxDecoration(
                  color: bank.isSelected ? null : Colors.transparent,
                  gradient: bank.isSelected
                      ? const LinearGradient(
                          colors: [
                            Color(0xFFFFFFFF),
                            Color(0xFF5BA1F7),
                            Color(0xFF031E6B),
                            Color(0xFF241714),
                          ],
                          stops: [0.0, 0.25, 0.7, 1.0],
                          begin: Alignment.topLeft,
                          end: Alignment.bottomRight,
                        )
                      : null,
                  border: bank.isSelected
                      ? null
                      : Border.all(
                          color: const Color(0xFFCBD5E1),
                          width: 1.5,
                        ),
                ),
                child: bank.isSelected
                    ? const Icon(Icons.check, color: Colors.white, size: 12)
                    : null,
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildFooter() {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16.0, horizontal: 24.0),
      child: FittedBox(
        fit: BoxFit.scaleDown,
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Text(
              'POWERED BY RBI-REGULATED ACCOUNT AGGREGATOR ',
              style: TextStyle(
                fontFamily: 'DMSans',
                fontSize: 9,
                color: Color(0xFF9CA3AF),
                fontWeight: FontWeight.w600,
              ),
            ),
            const Icon(
              Icons.change_history_rounded,
              color: Color(0xFF1E3A8A),
              size: 11,
            ),
            const SizedBox(width: 2),
            const Text(
              'FINARKEIN',
              style: TextStyle(
                fontFamily: 'DMSans',
                fontSize: 11,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1E3A8A),
                letterSpacing: 0.5,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Dotted horizontal line separator
class _DottedLinePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = const Color(0xFFCBD5E1)
      ..strokeWidth = 1.0;

    double startX = 0;
    const dashWidth = 4.0;
    const dashSpace = 4.0;
    while (startX < size.width) {
      canvas.drawLine(Offset(startX, 0), Offset(startX + dashWidth, 0), paint);
      startX += dashWidth + dashSpace;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

/// Consent Info Bottom Sheet (Image 2)
class _ConsentBottomSheet extends StatelessWidget {
  final VoidCallback onUnderstood;

  const _ConsentBottomSheet({required this.onUnderstood});

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: Color(0xFFFFFFFF),
        borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
      ),
      padding: const EdgeInsets.fromLTRB(24, 20, 24, 28),
      child: SafeArea(
        top: false,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              // Drag handle
              Center(
                child: Container(
                  width: 40,
                  height: 4,
                  decoration: BoxDecoration(
                    color: const Color(0xFFE2E8F0),
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
              const SizedBox(height: 20),

              // Title
              const Text(
                'DETAILS OF YOUR APPROVAL',
                style: TextStyle(
                  fontFamily: 'DMSans',
                  color: Color(0xFF9CA3AF),
                  fontSize: 10,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 0.8,
                ),
              ),
              const SizedBox(height: 16),
              const Divider(color: Color(0xFFE2E8F0), height: 1),
              const SizedBox(height: 16),

              // Details List
              _buildDetailRow(
                label: 'APPROVAL REQUESTED ON',
                value: '6 June 2026.',
              ),
              _buildDetailRow(
                label: 'PURPOSE',
                value:
                    'To generate insights based on your overall finances and provide incidental recommendations, if any.',
              ),
              _buildDetailRow(
                label: 'BALANCE AND TRANSACTIONS UPDATED',
                value: 'Upto 45 times per month.',
              ),
              _buildDetailRow(
                label: 'APPROVAL VALID FOR',
                value: '6 June 2026 - 6 June 2027.',
              ),
              _buildDetailRow(
                label: 'ACCOUNT DETAILS',
                value: 'Profile, summary, transactions.',
              ),
              _buildDetailRow(
                label: 'ACCESS PERIOD TO FIND INSIGHTS FOR YOU',
                value: '1 month.',
              ),
              _buildDetailRow(label: 'APPROVAL EXPIRY', value: '6 June 2027'),
              _buildDetailRow(
                label: 'ACCOUNT TYPES',
                value: 'Deposit.',
                isLast: true,
              ),

              const SizedBox(height: 28),

              // CTA Button (Understood)
              GestureDetector(
                onTap: onUnderstood,
                child: Container(
                  width: double.infinity,
                  padding: const EdgeInsets.symmetric(vertical: 15),
                  decoration: const BoxDecoration(
                    gradient: LinearGradient(
                      colors: [
                        Color(0xFFFFFFFF),
                        Color(0xFF5BA1F7),
                        Color(0xFF031E6B),
                        Color(0xFF241714),
                      ],
                      stops: [0.0, 0.25, 0.7, 1.0],
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                    ),
                  ),
                  child: const Stack(
                    alignment: Alignment.center,
                    children: [
                      Text(
                        'UNDERSTOOD',
                        style: TextStyle(
                          fontFamily: 'DMSans',
                          color: Colors.white,
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          letterSpacing: 1.0,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildDetailRow({
    required String label,
    required String value,
    bool isLast = false,
  }) {
    return Padding(
      padding: EdgeInsets.only(bottom: isLast ? 0 : 16.0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(
              fontFamily: 'DMSans',
              color: Color(0xFF9CA3AF),
              fontSize: 10,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 3),
          Text(
            value,
            style: const TextStyle(
              fontFamily: 'DMSans',
              color: Color(0xFF0F172A),
              fontSize: 11,
              fontWeight: FontWeight.w600,
              height: 1.3,
            ),
          ),
        ],
      ),
    );
  }
}
