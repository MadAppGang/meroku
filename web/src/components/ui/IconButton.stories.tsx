import type { Meta, StoryObj } from '@storybook/react';
import { Plus, X, Settings, Maximize2, Minimize2 } from 'lucide-react';
import { IconButton } from './IconButton';

const meta: Meta<typeof IconButton> = {
  title: 'UI/IconButton',
  component: IconButton,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
    },
  },
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    icon: <Settings className="w-5 h-5" />,
  },
};

export const Small: Story = {
  args: {
    icon: <X className="w-4 h-4" />,
    size: 'sm',
  },
};

export const Large: Story = {
  args: {
    icon: <Plus className="w-6 h-6" />,
    size: 'lg',
  },
};

export const ZoomControls: Story = {
  render: () => (
    <div className="flex gap-2">
      <IconButton icon={<Minimize2 className="w-5 h-5" />} />
      <IconButton icon={<Maximize2 className="w-5 h-5" />} />
    </div>
  ),
};