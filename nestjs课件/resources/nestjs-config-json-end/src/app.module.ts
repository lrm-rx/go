import { Module } from '@nestjs/common';
import { UserModule } from './user/user.module';
import * as config from 'config';

console.log(config.get('database'));

@Module({
  imports: [UserModule],
  controllers: [],
  providers: [],
})
export class AppModule {}
