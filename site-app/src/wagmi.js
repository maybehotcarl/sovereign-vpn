import { getDefaultConfig } from '@rainbow-me/rainbowkit';
import {
  coinbaseWallet,
  injectedWallet,
  metaMaskWallet,
  rabbyWallet,
  walletConnectWallet,
} from '@rainbow-me/rainbowkit/wallets';
import { mainnet, sepolia } from 'wagmi/chains';

const chains = import.meta.env.VITE_CHAIN === 'sepolia' ? [sepolia] : [mainnet];
const appUrl = import.meta.env.PROD ? 'https://6529vpn.io' : 'http://127.0.0.1:5173';

export const config = getDefaultConfig({
  appName: '6529 VPN',
  appDescription: 'Direct and anonymous WireGuard access for Memes holders.',
  appUrl,
  projectId: 'f5a95088cbbb27fd501c70138334ed22',
  chains,
  ssr: false,
  wallets: [
    {
      groupName: 'Recommended',
      wallets: [
        rabbyWallet,
        metaMaskWallet,
        coinbaseWallet,
        injectedWallet,
      ],
    },
    {
      groupName: 'Other',
      wallets: [walletConnectWallet],
    },
  ],
});
