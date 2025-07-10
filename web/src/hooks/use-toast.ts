import { toast as sonnerToast } from 'sonner';

export interface ToastOptions {
  title?: string;
  description?: string;
  variant?: 'default' | 'destructive';
  duration?: number;
}

export function useToast() {
  const toast = (options: ToastOptions) => {
    const { title, description, variant = 'default', duration = 5000 } = options;
    
    const message = title || description || '';
    const descriptionText = title && description ? description : undefined;
    
    if (variant === 'destructive') {
      sonnerToast.error(message, {
        description: descriptionText,
        duration,
      });
    } else {
      sonnerToast.success(message, {
        description: descriptionText,
        duration,
      });
    }
  };

  return { toast };
}