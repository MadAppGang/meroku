import type { Meta, StoryObj } from '@storybook/react';
import { AddComponentModal } from './AddComponentModal';
import { 
  Server, 
  Layers, 
  Database, 
  HardDrive, 
  Shield, 
  Mail, 
  Container 
} from 'lucide-react';

const meta: Meta<typeof AddComponentModal> = {
  title: 'Services/AddComponentModal',
  component: AddComponentModal,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

const componentTypes = [
  { type: "backend" as const, label: "Backend Service", icon: Server },
  { type: "frontend" as const, label: "Frontend Service", icon: Layers },
  { type: "database" as const, label: "PostgreSQL", icon: Database },
  { type: "redis" as const, label: "Redis Cache", icon: HardDrive },
  { type: "cognito" as const, label: "Cognito Auth", icon: Shield },
  { type: "ses" as const, label: "SES Email", icon: Mail },
  { type: "sqs" as const, label: "SQS Queue", icon: Container },
];

export const Open: Story = {
  args: {
    isOpen: true,
    onClose: () => console.log('Close modal'),
    componentTypes,
    onAddComponent: (type) => console.log('Add component:', type),
  },
};

export const Closed: Story = {
  args: {
    isOpen: false,
    onClose: () => {},
    componentTypes,
    onAddComponent: () => {},
  },
};