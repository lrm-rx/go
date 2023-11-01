import { defineAbility } from '@casl/ability';

const user = {
  id: 1,
  isAdmin: false,
};

class Post {
  constructor(attrs) {
    Object.assign(this, attrs);
  }
}

const ability = defineAbility((can, cannot) => {
  can('read', 'all');

  can('update', 'Post', ['content'], { isPublished: false, author: user.id });

  // cannot('update', 'Post', { isPublished: true })
  cannot('delete', 'Post');

  if (user.isAdmin) {
    can('update', 'Post');
    can('delete', 'Post');
  }
});

const somePost = new Post({ author: 1, isPublished: false });

const flag = ability.can('update', somePost, 'author');
flag;
