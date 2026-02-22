export const SESSION_MANAGER_ADDRESS = import.meta.env.VITE_SESSION_MANAGER || '';

export const SESSION_MANAGER_ABI = [
  {
    inputs: [
      { name: 'node', type: 'address' },
      { name: 'duration', type: 'uint256' },
    ],
    name: 'openSession',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'payable',
    type: 'function',
  },
  {
    inputs: [{ name: 'duration', type: 'uint256' }],
    name: 'calculatePrice',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'view',
    type: 'function',
  },
  {
    inputs: [{ name: 'user', type: 'address' }],
    name: 'getActiveSessionId',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'view',
    type: 'function',
  },
];

export const SUBSCRIPTION_MANAGER_ADDRESS = import.meta.env.VITE_SUBSCRIPTION_MANAGER || '';

export const SUBSCRIPTION_MANAGER_ABI = [
  {
    inputs: [
      { name: 'node', type: 'address' },
      { name: 'tierId', type: 'uint8' },
    ],
    name: 'subscribe',
    outputs: [],
    stateMutability: 'payable',
    type: 'function',
  },
  {
    inputs: [
      { name: 'tierId', type: 'uint8' },
      { name: 'node', type: 'address' },
    ],
    name: 'renewSubscription',
    outputs: [],
    stateMutability: 'payable',
    type: 'function',
  },
  {
    inputs: [{ name: 'user', type: 'address' }],
    name: 'hasActiveSubscription',
    outputs: [{ name: '', type: 'bool' }],
    stateMutability: 'view',
    type: 'function',
  },
  {
    inputs: [{ name: 'user', type: 'address' }],
    name: 'getSubscription',
    outputs: [
      {
        components: [
          { name: 'user', type: 'address' },
          { name: 'node', type: 'address' },
          { name: 'payment', type: 'uint256' },
          { name: 'startedAt', type: 'uint256' },
          { name: 'expiresAt', type: 'uint256' },
          { name: 'tier', type: 'uint8' },
        ],
        name: '',
        type: 'tuple',
      },
    ],
    stateMutability: 'view',
    type: 'function',
  },
];
