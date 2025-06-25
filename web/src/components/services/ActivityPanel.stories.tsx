import type { Meta, StoryObj } from '@storybook/react';
import { ActivityPanel } from './ActivityPanel';

const meta: Meta<typeof ActivityPanel> = {
  title: 'Services/ActivityPanel',
  component: ActivityPanel,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <div className="h-96 bg-gray-900 flex justify-end">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Open: Story = {
  args: {
    isOpen: true,
    onClose: () => console.log('Close activity panel'),
  },
};

export const Closed: Story = {
  args: {
    isOpen: false,
    onClose: () => {},
  },
};